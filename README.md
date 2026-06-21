# svcworkers

[![codecov](https://codecov.io/gh/alesr/svcworkers/graph/badge.svg?token=KEpvy1bjzn)](https://codecov.io/gh/alesr/svcworkers)

Reusable [svc.Worker](https://github.com/alesr/svc) implementations for Go services.

Each worker is a standalone Go module with its own `go.mod`.

## Workers

| Module | Package | Implements | Description |
|--------|---------|-----------|-------------|
| `pgworker` | `pgworker` | Worker + Aliver + Healther | PostgreSQL connection pool with configurable limits, ping |
| `redisworker` | `redisworker` | Worker + Aliver + Healther | Redis client with configurable pool, ping |
| `grpcworker` | `grpcworker` | Worker + Aliver + Healther | gRPC server with health protocol and reflection |
| `telemetryworker` | `telemetryworker` | Worker | OpenTelemetry SDK lifecycle (metrics, tracing, optional log export) |
| `httpserverworker` | `httpserverworker` | Worker + Aliver + Healther | HTTP server wrapping any `http.Handler` |

## Usage

```go
package main

import (
    "github.com/alesr/svc"
    "github.com/alesr/svcworkers/pgworker"
    "github.com/alesr/svcworkers/httpserverworker"
)

func main() {
    s, err := svc.New("my-service", "1.0.0",
        svc.WithHTTPServer("9090"),
        svc.WithHealthz(),
    )
    svc.MustInit(s, err)

    s.AddWorker("postgresql", pgworker.New(
        "postgres://localhost:5432/db?sslmode=disable",
        pgworker.WithMaxOpenConns(20),
    ))

    s.AddWorker("http-server", httpserverworker.New(":8080", s.Router))

    s.Run()
}
```

## Writing a custom worker

```go
type MyWorker struct {
    done chan struct{}
}

func (w *MyWorker) Init(*slog.Logger) error { return nil }
func (w *MyWorker) Run() error              { <-w.done; return nil }
func (w *MyWorker) Terminate() error        { close(w.done); return nil }
```
