//go:build integration

package trip_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres/transaction"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
	"github.com/kodmiq/TripGo-Avito-/internal/repository/idempotency"
	triprepo "github.com/kodmiq/TripGo-Avito-/internal/repository/trip"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const queryTimeout = 5 * time.Second

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("tripgo"),
		postgres.WithUsername("tripgo"),
		postgres.WithPassword("tripgo"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: start postgres: %v\n", err)
		os.Exit(1)
	}

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: connection string: %v\n", err)
		_ = pg.Terminate(ctx)
		os.Exit(1)
	}
	pool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: pool: %v\n", err)
		_ = pg.Terminate(ctx)
		os.Exit(1)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	if err := migrate(ctx, sqlDB); err != nil {
		fmt.Fprintf(os.Stderr, "integration: migrate: %v\n", err)
		closeSQL(sqlDB)
		pool.Close()
		_ = pg.Terminate(ctx)
		os.Exit(1)
	}

	code := m.Run()
	closeSQL(sqlDB)
	pool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

func closeSQL(db *sql.DB) {
	if err := db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "integration: close db: %v\n", err)
	}
}

func migrate(ctx context.Context, db *sql.DB) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, dir)
}

func migrationsDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate migrations")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")), nil
}

func cleanup(t *testing.T, ids ...uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		for _, id := range ids {
			if _, err := pool.Exec(ctx, "DELETE FROM trips WHERE id = $1", id); err != nil {
				t.Errorf("cleanup trip %s: %v", id, err)
			}
		}
	})
}

func sample(driverID uuid.UUID) model.Trip {
	return model.Trip{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		DriverID:  driverID,
		StartLat:  59.9398,
		StartLon:  30.3146,
		EndLat:    59.9290,
		EndLon:    30.3626,
		Price:     1450,
		Status:    string(domain.StatusActive),
		StartedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestSaveGetAndFinish(t *testing.T) {
	ctx := context.Background()
	repo := triprepo.New(pool, queryTimeout)
	history := triprepo.NewHistory(pool, queryTimeout)
	tx := transaction.New(pool, queryTimeout, slog.New(slog.DiscardHandler))
	draft := sample(uuid.New())
	cleanup(t, draft.ID)

	err := tx.Do(ctx, func(ctx context.Context) error {
		stored, err := repo.Save(ctx, draft)
		if err != nil {
			return err
		}
		return history.Save(ctx, model.StatusChange{
			TripID:    stored.ID,
			ToStatus:  string(domain.StatusActive),
			Reason:    "trip created",
			ChangedAt: stored.StartedAt,
		})
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.Get(ctx, draft.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DriverID != draft.DriverID || got.Status != string(domain.StatusActive) || got.FinishedAt != nil {
		t.Fatalf("get = %+v", got)
	}

	var historyRows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id = $1", draft.ID).Scan(&historyRows); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if historyRows != 1 {
		t.Fatalf("history rows = %d", historyRows)
	}

	finishedAt := draft.StartedAt.Add(time.Minute)
	completed, ok, err := repo.MarkCompleted(ctx, draft.ID, finishedAt)
	if err != nil || !ok {
		t.Fatalf("finish ok=%v err=%v", ok, err)
	}
	if completed.Status != string(domain.StatusCompleted) || completed.FinishedAt == nil || !completed.FinishedAt.Equal(finishedAt) {
		t.Fatalf("completed = %+v", completed)
	}
	if _, ok, err := repo.MarkCompleted(ctx, draft.ID, finishedAt.Add(time.Minute)); err != nil || ok {
		t.Fatalf("second finish ok=%v err=%v", ok, err)
	}
}

func TestDriverBusy(t *testing.T) {
	ctx := context.Background()
	repo := triprepo.New(pool, queryTimeout)
	driverID := uuid.New()
	first := sample(driverID)
	second := sample(driverID)
	cleanup(t, first.ID, second.ID)

	if _, err := repo.Save(ctx, first); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if _, err := repo.Save(ctx, second); !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("second save = %v, want ErrDriverBusy", err)
	}
}

func TestMissingTrip(t *testing.T) {
	repo := triprepo.New(pool, queryTimeout)
	_, err := repo.Get(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("Get() = %v, want ErrTripNotFound", err)
	}
}

func TestInsertRollsBackBothTables(t *testing.T) {
	ctx := context.Background()
	repo := triprepo.New(pool, queryTimeout)
	history := triprepo.NewHistory(pool, queryTimeout)
	keys := idempotency.New(pool, queryTimeout)
	tx := transaction.New(pool, queryTimeout, slog.New(slog.DiscardHandler))
	draft := sample(uuid.New())
	cleanup(t, draft.ID)
	key := uuid.New()
	forced := errors.New("forced rollback")

	err := tx.Do(ctx, func(ctx context.Context) error {
		now := draft.StartedAt
		if err := keys.Insert(ctx, model.IdempotencyRecord{
			Key: key, RequestHash: "hash", TripID: draft.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			return err
		}
		stored, err := repo.Save(ctx, draft)
		if err != nil {
			return err
		}
		if err := history.Save(ctx, model.StatusChange{
			TripID: stored.ID, ToStatus: string(domain.StatusActive), Reason: "trip created", ChangedAt: stored.StartedAt,
		}); err != nil {
			return err
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("Do() = %v, want forced rollback", err)
	}

	if _, err := repo.Get(ctx, draft.ID); !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("trip after rollback = %v", err)
	}
	var historyRows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id = $1", draft.ID).Scan(&historyRows); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if historyRows != 0 {
		t.Fatalf("history rows after rollback = %d", historyRows)
	}
	if _, err := keys.Get(ctx, key); !errors.Is(err, domain.ErrIdempotencyKeyNotFound) {
		t.Fatalf("idempotency key after rollback = %v", err)
	}
}
