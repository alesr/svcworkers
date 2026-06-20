package httpserverworker_test

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alesr/svcworkers/httpserverworker"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestHTTPServerWorker(t *testing.T) {
	t.Run("before run", func(t *testing.T) {
		t.Parallel()

		w := httpserverworker.New(":0", http.NewServeMux())
		require.NoError(t, w.Init(nil))
		require.Error(t, w.Alive())
		require.Error(t, w.Healthy())
	})

	t.Run("lifecycle", func(t *testing.T) {
		t.Parallel()

		port := freePort(t)
		addr := fmt.Sprintf("127.0.0.1:%d", port)

		mux := http.NewServeMux()
		mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pong"))
		})

		worker := httpserverworker.New(addr, mux)
		require.NoError(t, worker.Init(nil))

		go func() {
			_ = worker.Run()
		}()
		t.Cleanup(func() { _ = worker.Terminate() })

		require.Eventually(t, func() bool {
			return worker.Alive() == nil
		}, time.Second, 10*time.Millisecond)

		resp, err := http.Get(fmt.Sprintf("http://%s/ping", addr))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		require.NoError(t, worker.Healthy())
	})

	t.Run("terminate stops server", func(t *testing.T) {
		t.Parallel()

		port := freePort(t)
		addr := fmt.Sprintf("127.0.0.1:%d", port)

		worker := httpserverworker.New(addr, http.NewServeMux())
		require.NoError(t, worker.Init(nil))

		go func() {
			_ = worker.Run()
		}()

		require.Eventually(t, func() bool {
			return worker.Alive() == nil
		}, time.Second, 10*time.Millisecond)

		require.NoError(t, worker.Terminate())
		require.Error(t, worker.Alive())
		require.Error(t, worker.Healthy())
	})

	t.Run("options", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			opts []httpserverworker.Option
		}{
			{
				name: "custom timeouts",
				opts: []httpserverworker.Option{
					httpserverworker.WithReadTimeout(5 * time.Second),
					httpserverworker.WithReadHeaderTimeout(2 * time.Second),
					httpserverworker.WithWriteTimeout(10 * time.Second),
					httpserverworker.WithIdleTimeout(30 * time.Second),
					httpserverworker.WithShutdownTimeout(15 * time.Second),
				},
			},
			{name: "minimal", opts: nil},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := httpserverworker.New(":8080", http.NewServeMux(), tt.opts...)
				require.NotNil(t, w)
			})
		}
	})
}
