package telemetryworker_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/alesr/svcworkers/telemetryworker"
)

func TestTelemetryWorker(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		w := telemetryworker.New(false, "", "test-service")
		require.NoError(t, w.Init(nil))
		require.NoError(t, w.Terminate())
	})

	t.Run("enabled with otel collector", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping integration test")
		}

		ctx := context.Background()

		req := testcontainers.ContainerRequest{
			Image:        "otel/opentelemetry-collector-contrib:latest",
			ExposedPorts: []string{"4317/tcp"},
			WaitingFor:   wait.ForListeningPort("4317/tcp"),
		}

		otelC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		require.NoError(t, err)
		testcontainers.CleanupContainer(t, otelC)

		host, err := otelC.Host(ctx)
		require.NoError(t, err)

		port, err := otelC.MappedPort(ctx, "4317")
		require.NoError(t, err)

		endpoint := fmt.Sprintf("%s:%s", host, port.Port())

		require.Eventually(t, func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, err := grpc.NewClient(endpoint,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return false
			}

			defer conn.Close()
			conn.Connect()

			for {
				s := conn.GetState()
				if s == connectivity.Ready {
					return true
				}

				if !conn.WaitForStateChange(ctx, s) {
					return false
				}
			}
		}, 15*time.Second, 200*time.Millisecond, "collector gRPC not ready")

		w := telemetryworker.New(true, endpoint, "test-svc",
			telemetryworker.WithMetricInterval(5*time.Second),
			telemetryworker.WithBatchTimeout(5*time.Second),
		)
		require.NoError(t, w.Init(nil))

		go func() {
			_ = w.Run()
		}()

		require.NoError(t, w.Terminate())
	})
}
