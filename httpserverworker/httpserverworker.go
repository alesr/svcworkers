package httpserverworker

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

	"github.com/alesr/svc"
)

var (
	_ svc.Worker   = (*Worker)(nil)
	_ svc.Aliver   = (*Worker)(nil)
	_ svc.Healther = (*Worker)(nil)
)

type Option func(*Worker)

func WithReadTimeout(d time.Duration) Option     { return func(w *Worker) { w.readTimeout = d } }
func WithWriteTimeout(d time.Duration) Option    { return func(w *Worker) { w.writeTimeout = d } }
func WithIdleTimeout(d time.Duration) Option     { return func(w *Worker) { w.idleTimeout = d } }
func WithShutdownTimeout(d time.Duration) Option { return func(w *Worker) { w.shutdownTimeout = d } }
func WithReadHeaderTimeout(d time.Duration) Option {
	return func(w *Worker) { w.readHeaderTimeout = d }
}

type Worker struct {
	addr    string
	handler http.Handler
	server  *http.Server
	lis     net.Listener
	lisMu   sync.Mutex
	logger  *slog.Logger

	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	shutdownTimeout   time.Duration

	running atomic.Bool
	stopped atomic.Bool
}

func New(addr string, handler http.Handler, opts ...Option) *Worker {
	w := &Worker{
		addr:              addr,
		handler:           handler,
		readHeaderTimeout: 5 * time.Second,
		shutdownTimeout:   10 * time.Second,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Init(logger *slog.Logger) error {
	if logger != nil {
		w.logger = logger.With("worker", "http-server")
	}
	w.server = &http.Server{
		Addr:              w.addr,
		Handler:           w.handler,
		ReadTimeout:       w.readTimeout,
		ReadHeaderTimeout: w.readHeaderTimeout,
		WriteTimeout:      w.writeTimeout,
		IdleTimeout:       w.idleTimeout,
	}
	return nil
}

func (w *Worker) Run() error {
	lis, err := net.Listen("tcp", w.addr)
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
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
		return err
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
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}

func (w *Worker) Alive() error {
	if !w.running.Load() {
		return errors.New("http server not running")
	}
	return nil
}

func (w *Worker) Healthy() error {
	w.lisMu.Lock()
	lis := w.lis
	w.lisMu.Unlock()

	if lis == nil {
		return errors.New("http server not accepting connections")
	}

	conn, err := net.DialTimeout("tcp", lis.Addr().String(), 2*time.Second)
	if err != nil {
		return fmt.Errorf("http server not accepting connections: %w", err)
	}
	conn.Close()
	return nil
}
