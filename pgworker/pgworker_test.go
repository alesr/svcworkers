package pgworker_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/alesr/svcworkers/pgworker"
)

func TestPGWorker(t *testing.T) {
	t.Run("unit", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			dsn   string
			opts  []pgworker.Option
			check func(*testing.T, *pgworker.Worker)
		}{
			{
				name: "defaults",
				dsn:  "postgres://localhost:5432/test?sslmode=disable",
				check: func(t *testing.T, w *pgworker.Worker) {
					require.NotNil(t, w)
					require.Error(t, w.Alive())
				},
			},
			{
				name: "custom options",
				dsn:  "postgres://localhost:5432/test?sslmode=disable",
				opts: []pgworker.Option{
					pgworker.WithMaxOpenConns(10),
					pgworker.WithMaxIdleConns(2),
					pgworker.WithConnMaxLifetime(1 * time.Minute),
					pgworker.WithConnMaxIdleTime(30 * time.Second),
					pgworker.WithPingTimeout(2 * time.Second),
				},
				check: func(t *testing.T, w *pgworker.Worker) {
					require.NotNil(t, w)
				},
			},
			{
				name: "invalid dsn fails init",
				dsn:  "invalid://",
				check: func(t *testing.T, w *pgworker.Worker) {
					require.Error(t, w.Init(nil))
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := pgworker.New(tt.dsn, tt.opts...)
				tt.check(t, w)
			})
		}
	})

	t.Run("integration", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping integration test")
		}

		ctx := context.Background()

		pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
			tcpostgres.WithDatabase("testdb"),
		)
		testcontainers.CleanupContainer(t, pg)
		require.NoError(t, err)

		dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
		require.NoError(t, err)

		var w *pgworker.Worker
		require.Eventually(t, func() bool {
			w = pgworker.New(dsn)
			return w.Init(nil) == nil
		}, 5*time.Second, 200*time.Millisecond)

		require.NoError(t, w.Alive())
		require.NoError(t, w.Healthy())

		require.NoError(t, w.Terminate())
		require.Error(t, w.Alive())
	})
}
