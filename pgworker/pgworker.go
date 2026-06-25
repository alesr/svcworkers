package pgworker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/lib/pq"

	"github.com/alesr/svc"
)

var (
	_ svc.Worker   = (*Worker)(nil)
	_ svc.Aliver   = (*Worker)(nil)
	_ svc.Healther = (*Worker)(nil)
)

type Option func(*Worker)

func WithMaxOpenConns(n int) Option              { return func(w *Worker) { w.maxOpenConns = n } }
func WithMaxIdleConns(n int) Option              { return func(w *Worker) { w.maxIdleConns = n } }
func WithConnMaxLifetime(d time.Duration) Option { return func(w *Worker) { w.connMaxLifetime = d } }
func WithConnMaxIdleTime(d time.Duration) Option { return func(w *Worker) { w.connMaxIdleTime = d } }
func WithPingTimeout(d time.Duration) Option     { return func(w *Worker) { w.pingTimeout = d } }

type Worker struct {
	dsn    string
	db     *sql.DB
	logger *slog.Logger
	done   chan struct{}

	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	connMaxIdleTime time.Duration
	pingTimeout     time.Duration
}

func New(dsn string, opts ...Option) *Worker {
	w := &Worker{
		dsn:             dsn,
		maxOpenConns:    25,
		maxIdleConns:    5,
		connMaxLifetime: 5 * time.Minute,
		connMaxIdleTime: 5 * time.Minute,
		pingTimeout:     5 * time.Second,
		done:            make(chan struct{}),
	}

	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Init(logger *slog.Logger) error {
	if logger != nil {
		w.logger = logger.With("worker", "postgresql")
	}

	db, err := sql.Open("postgres", w.dsn)
	if err != nil {
		return fmt.Errorf("could not open db: %w", err)
	}

	db.SetMaxOpenConns(w.maxOpenConns)
	db.SetMaxIdleConns(w.maxIdleConns)
	db.SetConnMaxLifetime(w.connMaxLifetime)
	db.SetConnMaxIdleTime(w.connMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), w.pingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("could not ping db: %w", err)
	}

	w.db = db
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

	if w.db != nil {
		err := w.db.Close()
		w.db = nil
		return err
	}
	return nil
}

func (w *Worker) Alive() error {
	if w.db == nil {
		return errors.New("database not initialized")
	}
	return nil
}

func (w *Worker) Healthy() error {
	ctx, cancel := context.WithTimeout(context.Background(), w.pingTimeout)
	defer cancel()
	return w.db.PingContext(ctx)
}

func (w *Worker) DB() *sql.DB { return w.db }
