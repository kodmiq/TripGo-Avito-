package container

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
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
	once   sync.Once
	close  func(context.Context)
}

func (a *App) Close(ctx context.Context) {
	if a.close == nil {
		return
	}
	a.once.Do(func() { a.close(ctx) })
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

type systemIDs struct{}

func (systemIDs) New() uuid.UUID {
	return uuid.New()
}

// New создаёт конкретные реализации и связывает их. Пул, телеметрия и репозитории рождаются только здесь.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	tel, err := telemetry.Setup(ctx, cfg)
	if err != nil {
		return nil, err
	}

	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DatabaseMaxConns,
		MinConns:        cfg.DatabaseMinConns,
		MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
		ConnectTimeout:  cfg.DatabaseConnectTimeout,
		Tracer:          tel.DB,
	})
	if err != nil {
		_ = tel.Shutdown(ctx)
		return nil, err
	}

	app, err := wire(cfg, tel, pool)
	if err != nil {
		pool.Close()
		_ = tel.Shutdown(ctx)
		return nil, err
	}
	return app, nil
}

func wire(cfg config.Config, tel telemetry.Providers, pool *pgxpool.Pool) (*App, error) {
	log := tel.Logger
	trips := trip.New(pool, cfg.DatabaseQueryTimeout)
	history := trip.NewHistory(pool, cfg.DatabaseQueryTimeout)
	keys := idempotency.New(pool, cfg.DatabaseQueryTimeout)
	tx := transaction.New(pool, cfg.DatabaseQueryTimeout, log)
	clock := systemClock{}
	meter := tel.Meter.Meter("trip-service")

	if err := telemetry.WatchActiveTrips(meter, log, trips.CountActive); err != nil {
		return nil, err
	}

	createTrip := createuc.New(tx, trips, history, keys, cfg.IdempotencyTTL, clock, systemIDs{}, log, tel.Trips)
	getTrip := getuc.New(trips)
	finishTrip := finishuc.New(tx, trips, history, clock, log, tel.Trips)

	handler, err := server.New(
		httpcreate.New(createTrip, log),
		httpget.New(getTrip, log),
		httpfinish.New(finishTrip, log),
		health.New(),
		ready.New(pool, cfg.DatabaseConnectTimeout, log),
		log,
		tel.Tracer,
		meter,
	)
	if err != nil {
		return nil, err
	}

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
		close: func(ctx context.Context) {
			if err := tel.Shutdown(ctx); err != nil {
				log.ErrorContext(ctx, "telemetry shutdown", "err", err)
			}
			pool.Close()
		},
	}, nil
}
