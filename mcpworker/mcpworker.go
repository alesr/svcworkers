package mcpworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/alesr/svc"
)

var (
	_ svc.Worker   = (*Worker)(nil)
	_ svc.Aliver   = (*Worker)(nil)
	_ svc.Healther = (*Worker)(nil)
)

type PingInput struct {
	Message string `json:"message" jsonschema:"Message to echo back"`
}

type PingOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Option func(*Worker)

func WithAddr(addr string) Option {
	return func(w *Worker) { w.addr = addr }
}

func WithInstructions(instr string) Option {
	return func(w *Worker) { w.instructions = instr }
}

func WithServerName(name string) Option {
	return func(w *Worker) { w.serverName = name }
}

func WithServerVersion(v string) Option {
	return func(w *Worker) { w.serverVersion = v }
}

func WithToolRegistrar(reg func(*mcp.Server)) Option {
	return func(w *Worker) { w.toolRegs = append(w.toolRegs, reg) }
}

func WithShutdownTimeout(d time.Duration) Option {
	return func(w *Worker) { w.shutdownTimeout = d }
}

type Worker struct {
	addr            string
	instructions    string
	serverName      string
	serverVersion   string
	server          *http.Server
	handler         http.Handler
	lis             net.Listener
	lisMu           sync.Mutex
	logger          *slog.Logger
	shutdownTimeout time.Duration
	toolRegs        []func(*mcp.Server)
	running         atomic.Bool
	stopped         atomic.Bool
}

func New(opts ...Option) *Worker {
	w := &Worker{
		addr:            ":0",
		serverName:      "mcp-server",
		serverVersion:   "0.1.0",
		instructions:    "MCP server. Provides tools to interact with the service.",
		shutdownTimeout: 10 * time.Second,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Init(logger *slog.Logger) error {
	if logger != nil {
		w.logger = logger.With("worker", "mcp")
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    w.serverName,
		Version: w.serverVersion,
	}, &mcp.ServerOptions{
		Instructions: w.instructions,
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "ping",
		Description: "Echoes back the provided message. Use this to verify the MCP server is working correctly.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in PingInput) (*mcp.CallToolResult, PingOutput, error) {
		return &mcp.CallToolResult{}, PingOutput{
			Status:  "ok",
			Message: in.Message,
		}, nil
	})

	for _, reg := range w.toolRegs {
		reg(mcpServer)
	}

	w.handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{})

	w.server = &http.Server{
		Handler:           w.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return nil
}

func (w *Worker) Run() error {
	lis, err := net.Listen("tcp", w.addr)
	if err != nil {
		return fmt.Errorf("mcp listen: %w", err)
	}
	w.lisMu.Lock()
	w.lis = lis
	w.lisMu.Unlock()
	w.running.Store(true)
	defer w.running.Store(false)

	if w.logger != nil {
		w.logger.Info("listening", "address", lis.Addr().String())
	}

	if err := w.server.Serve(lis); err != http.ErrServerClosed {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}

func (w *Worker) Terminate() error {
	if !w.stopped.CompareAndSwap(false, true) {
		return nil
	}
	w.running.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), w.shutdownTimeout)
	defer cancel()

	if err := w.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("mcp shutdown: %w", err)
	}

	w.lisMu.Lock()
	defer w.lisMu.Unlock()
	if w.lis != nil {
		w.lis.Close()
	}
	return nil
}

func (w *Worker) Alive() error {
	if !w.running.Load() {
		return errors.New("mcp server not running")
	}
	return nil
}

func (w *Worker) Healthy() error {
	w.lisMu.Lock()
	lis := w.lis
	w.lisMu.Unlock()

	if lis == nil {
		return errors.New("MCP server not accepting connections")
	}

	conn, err := net.DialTimeout("tcp", lis.Addr().String(), 2*time.Second)
	if err != nil {
		return fmt.Errorf("MCP server not accepting connections: %w", err)
	}
	conn.Close()
	return nil
}

func (w *Worker) Addr() net.Addr {
	w.lisMu.Lock()
	defer w.lisMu.Unlock()
	if w.lis == nil {
		return nil
	}
	return w.lis.Addr()
}
