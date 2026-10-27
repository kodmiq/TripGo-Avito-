package container

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kodmiq/TripGo-Avito-/internal/config"
	"github.com/kodmiq/TripGo-Avito-/internal/http/health"
	"github.com/kodmiq/TripGo-Avito-/internal/http/ready"
	"github.com/kodmiq/TripGo-Avito-/internal/http/server"
	httpcreate "github.com/kodmiq/TripGo-Avito-/internal/http/trip/create"
	httpfinish "github.com/kodmiq/TripGo-Avito-/internal/http/trip/finish"
	httpget "github.com/kodmiq/TripGo-Avito-/internal/http/trip/get"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres/transaction"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/telemetry"
	"github.com/kodmiq/TripGo-Avito-/internal/repository/idempotency"
	"github.com/kodmiq/TripGo-Avito-/internal/repository/trip"
	createuc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/create"
	finishuc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/finish"
	getuc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/get"
)

// App — собранный процесс. main запускает Server и по остановке вызывает Close.
type App struct {
	Server *http.Server
	Logger *slog.Logger
	close  func()
}

func (a *App) Close() {
	if a.close != nil {
		a.close()
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

type systemIDs struct{}

func (systemIDs) New() uuid.UUID {
	return uuid.New()
}

// New создаёт конкретные реализации и связывает их. Пул и репозитории рождаются только здесь.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	log := telemetry.NewLogger(cfg.LogLevel)

	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DatabaseMaxConns,
		MinConns:        cfg.DatabaseMinConns,
		MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
		ConnectTimeout:  cfg.DatabaseConnectTimeout,
	})
	if err != nil {
		return nil, err
	}

	app := wire(cfg, log, pool)
	return app, nil
}

func wire(cfg config.Config, log *slog.Logger, pool *pgxpool.Pool) *App {
	trips := trip.New(pool, cfg.DatabaseQueryTimeout)
	history := trip.NewHistory(pool, cfg.DatabaseQueryTimeout)
	keys := idempotency.New(pool, cfg.DatabaseQueryTimeout)
	tx := transaction.New(pool, cfg.DatabaseQueryTimeout, log)
	clock := systemClock{}

	createTrip := createuc.New(tx, trips, history, keys, cfg.IdempotencyTTL, clock, systemIDs{}, log)
	getTrip := getuc.New(trips)
	finishTrip := finishuc.New(tx, trips, history, clock, log)

	handler := server.New(
		httpcreate.New(createTrip, log),
		httpget.New(getTrip, log),
		httpfinish.New(finishTrip, log),
		health.New(),
		ready.New(pool, cfg.DatabaseConnectTimeout, log),
		log,
	)

	return &App{
		Logger: log,
		Server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTPReadTimeout,
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       cfg.HTTPIdleTimeout,
		},
		close: pool.Close,
	}
}
