package grpcworker

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/alesr/svc"
)

var (
	_ svc.Worker   = (*Worker)(nil)
	_ svc.Aliver   = (*Worker)(nil)
	_ svc.Healther = (*Worker)(nil)
)

type Option func(*Worker)

func WithServerOptions(opts ...gogrpc.ServerOption) Option {
	return func(w *Worker) { w.serverOpts = append(w.serverOpts, opts...) }
}

type Worker struct {
	port   string
	srv    *gogrpc.Server
	health *health.Server
	lis    net.Listener
	lisMu  sync.Mutex
	logger *slog.Logger

	serverOpts []gogrpc.ServerOption
	running    atomic.Bool
	stopped    atomic.Bool
}

func New(port string, opts ...Option) *Worker {
	w := &Worker{port: port}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Init(logger *slog.Logger) error {
	if logger != nil {
		w.logger = logger.With("worker", "grpc")
	}

	w.srv = gogrpc.NewServer(w.serverOpts...)

	w.health = health.NewServer()
	healthpb.RegisterHealthServer(w.srv, w.health)
	w.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(w.srv)

	return nil
}

func (w *Worker) Run() error {
	lis, err := net.Listen("tcp", ":"+w.port)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	w.lisMu.Lock()
	w.lis = lis
	w.lisMu.Unlock()

	w.running.Store(true)
	defer w.running.Store(false)

	return w.srv.Serve(lis)
}

func (w *Worker) Terminate() error {
	if !w.stopped.CompareAndSwap(false, true) {
		return nil
	}

	w.running.Store(false)
	w.srv.GracefulStop()

	w.lisMu.Lock()
	defer w.lisMu.Unlock()

	if w.lis != nil {
		w.lis.Close()
	}
	return nil
}

func (w *Worker) Alive() error {
	if !w.running.Load() {
		return errors.New("gRPC server not running")
	}
	return nil
}

func (w *Worker) Healthy() error {
	if !w.running.Load() {
		return errors.New("gRPC server not running")
	}
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
