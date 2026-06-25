package redisworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/alesr/svc"
)

var (
	_ svc.Worker   = (*Worker)(nil)
	_ svc.Aliver   = (*Worker)(nil)
	_ svc.Healther = (*Worker)(nil)
)

type Option func(*Worker)

func WithMinIdleConns(n int) Option              { return func(w *Worker) { w.minIdleConns = n } }
func WithPoolTimeout(d time.Duration) Option     { return func(w *Worker) { w.poolTimeout = d } }
func WithConnMaxLifetime(d time.Duration) Option { return func(w *Worker) { w.connMaxLifetime = d } }
func WithConnMaxIdleTime(d time.Duration) Option { return func(w *Worker) { w.connMaxIdleTime = d } }
func WithPingTimeout(d time.Duration) Option     { return func(w *Worker) { w.pingTimeout = d } }

type Worker struct {
	url    string
	client *redis.Client
	logger *slog.Logger
	done   chan struct{}

	minIdleConns    int
	poolTimeout     time.Duration
	connMaxLifetime time.Duration
	connMaxIdleTime time.Duration
	pingTimeout     time.Duration
}

func New(url string, opts ...Option) *Worker {
	w := Worker{
		url:             url,
		minIdleConns:    3,
		poolTimeout:     4 * time.Second,
		connMaxLifetime: 0,
		connMaxIdleTime: 5 * time.Minute,
		pingTimeout:     5 * time.Second,
		done:            make(chan struct{}),
	}

	for _, opt := range opts {
		opt(&w)
	}
	return &w
}

func (w *Worker) Init(logger *slog.Logger) error {
	if logger != nil {
		w.logger = logger.With("worker", "redis")
	}

	opts, err := redis.ParseURL(w.url)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}

	opts.MinIdleConns = w.minIdleConns
	opts.PoolTimeout = w.poolTimeout
	opts.ConnMaxLifetime = w.connMaxLifetime
	opts.ConnMaxIdleTime = w.connMaxIdleTime

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), w.pingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return fmt.Errorf("ping redis: %w", err)
	}

	w.client = client
	return nil
}

func (w *Worker) Run() error {
	<-w.done
	return nil
}

func (w *Worker) Terminate() error {
	select {
	case <-w.done:
		return nil
	default:
		close(w.done)
	}

	if w.client != nil {
		err := w.client.Close()
		w.client = nil
		return err
	}
	return nil
}

func (w *Worker) Alive() error {
	if w.client == nil {
		return errors.New("redis not initialized")
	}
	return nil
}

func (w *Worker) Healthy() error {
	ctx, cancel := context.WithTimeout(context.Background(), w.pingTimeout)
	defer cancel()

	if err := w.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("could not ping redis: %w", err)
	}
	return nil
}

func (w *Worker) Client() *redis.Client { return w.client }
