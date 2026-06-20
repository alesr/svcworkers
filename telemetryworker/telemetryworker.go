package telemetryworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/alesr/svc"
)

var _ svc.Worker = (*Worker)(nil)

type Worker struct {
	enabled      bool
	otlpEndpoint string
	serviceName  string
	shutdown     func(context.Context) error
	logger       *slog.Logger
	done         chan struct{}

	resourceAttrs  []attribute.KeyValue
	metricInterval time.Duration
	batchTimeout   time.Duration
	logExporter    bool
}

type Option func(*Worker)

func WithMetricInterval(d time.Duration) Option { return func(w *Worker) { w.metricInterval = d } }
func WithBatchTimeout(d time.Duration) Option   { return func(w *Worker) { w.batchTimeout = d } }
func WithLogExporter(enabled bool) Option       { return func(w *Worker) { w.logExporter = enabled } }
func WithResourceAttributes(attrs ...attribute.KeyValue) Option {
	return func(w *Worker) { w.resourceAttrs = append(w.resourceAttrs, attrs...) }
}

func New(enabled bool, otlpEndpoint, serviceName string, opts ...Option) *Worker {
	w := &Worker{
		enabled:        enabled,
		otlpEndpoint:   otlpEndpoint,
		serviceName:    serviceName,
		metricInterval: 3 * time.Second,
		batchTimeout:   3 * time.Second,
		done:           make(chan struct{}),
	}

	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Init(logger *slog.Logger) error {
	if logger != nil {
		w.logger = logger.With("worker", "telemetry")
	}

	if !w.enabled {
		w.shutdown = func(context.Context) error { return nil }
		return nil
	}

	var shutdownFuncs []func(context.Context) error

	shutdown := func(ctx context.Context) error {
		var err error
		for i := len(shutdownFuncs) - 1; i >= 0; i-- {
			err = errors.Join(err, shutdownFuncs[i](ctx))
		}
		return err
	}

	resAttrs := append(
		[]attribute.KeyValue{semconv.ServiceName(w.serviceName)},
		w.resourceAttrs...,
	)

	res, err := resource.New(context.Background(), resource.WithAttributes(resAttrs...))
	if err != nil {
		return fmt.Errorf("init new resource: %w", err)
	}

	meterProvider, err := newMeterProvider(w.otlpEndpoint, res, w.metricInterval)
	if err != nil {
		return fmt.Errorf("could not init meter provider: %w", err)
	}

	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	tracerProvider, err := newTracerProvider(w.otlpEndpoint, res, w.batchTimeout)
	if err != nil {
		return fmt.Errorf("could not init tracer provider: %w", err)
	}

	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if w.logExporter {
		logProvider, err := newLogProvider(w.otlpEndpoint, res, w.batchTimeout)
		if err != nil {
			return fmt.Errorf("could not init log provider: %w", err)
		}

		shutdownFuncs = append(shutdownFuncs, logProvider.Shutdown)
		logglobal.SetLoggerProvider(logProvider)
	}

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		return fmt.Errorf("runtime: %w", err)
	}

	w.shutdown = shutdown
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

	if w.shutdown != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return w.shutdown(ctx)
	}
	return nil
}

func newMeterProvider(endpoint string, res *resource.Resource, interval time.Duration) (*sdkmetric.MeterProvider, error) {
	host := extractHost(endpoint)

	exp, err := otlpmetricgrpc.New(context.Background(),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(host),
	)
	if err != nil {
		return nil, fmt.Errorf("metric exporter: %w", err)
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(interval))),
	), nil
}

func newTracerProvider(endpoint string, res *resource.Resource, timeout time.Duration) (*sdktrace.TracerProvider, error) {
	host := extractHost(endpoint)

	exp, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(host),
	)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(timeout)),
	), nil
}

func newLogProvider(endpoint string, res *resource.Resource, timeout time.Duration) (*sdklog.LoggerProvider, error) {
	host := extractHost(endpoint)

	exp, err := otlploggrpc.New(context.Background(),
		otlploggrpc.WithInsecure(),
		otlploggrpc.WithEndpoint(host),
	)
	if err != nil {
		return nil, fmt.Errorf("log exporter: %w", err)
	}

	return sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp, sdklog.WithExportMaxBatchSize(512))),
	), nil
}

func extractHost(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	if strings.HasPrefix(endpoint, "[") {
		if idx := strings.Index(endpoint, "]"); idx != -1 {
			host := endpoint[:idx+1]
			rest := endpoint[idx+1:]

			if rest != "" && rest[0] == ':' {
				host += rest
			}

			if u, err := url.Parse("http://" + host); err == nil {
				return u.Host
			}
		}
	}
	return endpoint
}
