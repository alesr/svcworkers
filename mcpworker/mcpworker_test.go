package mcpworker_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alesr/svcworkers/mcpworker"
)

func waitForAddr(t *testing.T, w *mcpworker.Worker) {
	t.Helper()
	require.Eventually(t, func() bool {
		return w.Addr() != nil
	}, time.Second, 10*time.Millisecond)
}

func TestMCPWorker(t *testing.T) {
	t.Run("before run", func(t *testing.T) {
		t.Parallel()

		w := mcpworker.New(mcpworker.WithAddr(":0"))
		require.NoError(t, w.Init(nil))
		require.Error(t, w.Alive())
		require.Error(t, w.Healthy())
	})

	t.Run("lifecycle", func(t *testing.T) {
		t.Parallel()

		w := mcpworker.New(mcpworker.WithAddr(":0"))
		require.NoError(t, w.Init(nil))

		go func() { _ = w.Run() }()
		t.Cleanup(func() { _ = w.Terminate() })

		waitForAddr(t, w)

		resp, err := http.Get(fmt.Sprintf("http://%s/", w.Addr().String()))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("terminate stops server", func(t *testing.T) {
		t.Parallel()

		w := mcpworker.New(mcpworker.WithAddr(":0"))
		require.NoError(t, w.Init(nil))

		go func() { _ = w.Run() }()

		waitForAddr(t, w)

		require.NoError(t, w.Terminate())
		require.Error(t, w.Alive())
		require.Error(t, w.Healthy())
	})

	t.Run("custom options", func(t *testing.T) {
		t.Parallel()

		w := mcpworker.New(
			mcpworker.WithAddr(":0"),
			mcpworker.WithInstructions("custom instructions"),
			mcpworker.WithServerName("custom-name"),
			mcpworker.WithServerVersion("2.0.0"),
		)
		require.NoError(t, w.Init(nil))

		go func() { _ = w.Run() }()
		t.Cleanup(func() { _ = w.Terminate() })

		waitForAddr(t, w)

		resp, err := http.Get(fmt.Sprintf("http://%s/", w.Addr().String()))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
