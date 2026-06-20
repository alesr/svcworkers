package redisworker_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/alesr/svcworkers/redisworker"
)

func TestRedisWorker(t *testing.T) {
	t.Run("unit", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			url   string
			opts  []redisworker.Option
			check func(*testing.T, *redisworker.Worker)
		}{
			{
				name: "defaults",
				url:  "redis://localhost:6379/0",
				check: func(t *testing.T, w *redisworker.Worker) {
					require.NotNil(t, w)
					require.Error(t, w.Alive())
				},
			},
			{
				name: "custom options",
				url:  "redis://localhost:6379/0",
				opts: []redisworker.Option{
					redisworker.WithMinIdleConns(5),
					redisworker.WithPoolTimeout(2 * time.Second),
					redisworker.WithConnMaxLifetime(1 * time.Minute),
					redisworker.WithConnMaxIdleTime(30 * time.Second),
					redisworker.WithPingTimeout(2 * time.Second),
				},
				check: func(t *testing.T, w *redisworker.Worker) {
					require.NotNil(t, w)
				},
			},
			{
				name: "invalid url fails init",
				url:  "invalid://",
				check: func(t *testing.T, w *redisworker.Worker) {
					require.Error(t, w.Init(nil))
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := redisworker.New(tt.url, tt.opts...)
				tt.check(t, w)
			})
		}
	})

	t.Run("integration", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping integration test")
		}

		ctx := context.Background()

		rc, err := tcredis.Run(ctx, "redis:7-alpine")
		testcontainers.CleanupContainer(t, rc)
		require.NoError(t, err)

		uri, err := rc.ConnectionString(ctx)
		require.NoError(t, err)

		w := redisworker.New(uri)
		require.NoError(t, w.Init(nil))

		require.NoError(t, w.Alive())
		require.NoError(t, w.Healthy())

		require.NoError(t, w.Terminate())
		require.Error(t, w.Alive())
	})
}
