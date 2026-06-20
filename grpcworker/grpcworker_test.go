package grpcworker_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	appgrpc "github.com/alesr/svcworkers/grpcworker"
)

func waitForAddr(t *testing.T, w *appgrpc.Worker) {
	t.Helper()
	require.Eventually(t, func() bool {
		return w.Addr() != nil
	}, time.Second, 10*time.Millisecond)
}

func TestGRPCWorker(t *testing.T) {
	t.Run("before run", func(t *testing.T) {
		t.Parallel()

		w := appgrpc.New("0")
		require.NoError(t, w.Init(nil))
		require.Error(t, w.Alive())
		require.Error(t, w.Healthy())
	})

	t.Run("health check", func(t *testing.T) {
		t.Parallel()

		w := appgrpc.New("0")
		require.NoError(t, w.Init(nil))

		go func() {
			_ = w.Run()
		}()
		t.Cleanup(func() { _ = w.Terminate() })

		waitForAddr(t, w)

		addr := w.Addr()
		require.NotNil(t, addr)

		conn, err := grpc.NewClient(addr.String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })

		client := healthpb.NewHealthClient(conn)
		resp, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
		require.NoError(t, err)
		require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
	})

	t.Run("after run", func(t *testing.T) {
		t.Parallel()

		w := appgrpc.New("0")
		require.NoError(t, w.Init(nil))

		go func() {
			_ = w.Run()
		}()
		t.Cleanup(func() { _ = w.Terminate() })

		require.Eventually(t, func() bool {
			return w.Alive() == nil
		}, time.Second, 10*time.Millisecond)
		require.NoError(t, w.Healthy())
	})

	t.Run("terminate stops server", func(t *testing.T) {
		t.Parallel()

		w := appgrpc.New("0")
		require.NoError(t, w.Init(nil))

		go func() {
			_ = w.Run()
		}()

		waitForAddr(t, w)
		require.NoError(t, w.Terminate())

		require.Error(t, w.Alive())
		require.Error(t, w.Healthy())
	})

	t.Run("server options", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			opts []grpc.ServerOption
		}{
			{
				name: "max message size",
				opts: []grpc.ServerOption{
					grpc.MaxRecvMsgSize(1024),
					grpc.MaxSendMsgSize(1024),
				},
			},
			{name: "no options", opts: nil},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := appgrpc.New("0",
					appgrpc.WithServerOptions(tt.opts...),
				)
				require.NoError(t, w.Init(nil))
			})
		}
	})
}
