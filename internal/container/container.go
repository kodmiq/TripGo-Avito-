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
	"github.com/kodmiq/TripGo-Avito-/internal/gateway/push"
	"github.com/kodmiq/TripGo-Avito-/internal/http/health"
	"github.com/kodmiq/TripGo-Avito-/internal/http/ready"
	"github.com/kodmiq/TripGo-Avito-/internal/http/server"
	httpadd "github.com/kodmiq/TripGo-Avito-/internal/http/trip/addposition"
	httpcreate "github.com/kodmiq/TripGo-Avito-/internal/http/trip/create"
	httpfinish "github.com/kodmiq/TripGo-Avito-/internal/http/trip/finish"
	httpget "github.com/kodmiq/TripGo-Avito-/internal/http/trip/get"
	httplist "github.com/kodmiq/TripGo-Avito-/internal/http/trip/listpositions"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres/transaction"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/telemetry"
	"github.com/kodmiq/TripGo-Avito-/internal/repository/idempotency"
	"github.com/kodmiq/TripGo-Avito-/internal/repository/position"
	"github.com/kodmiq/TripGo-Avito-/internal/repository/trip"
	adduc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/addposition"
	createuc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/create"
	finishuc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/finish"
	getuc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/get"
	listuc "github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/listpositions"
	"github.com/kodmiq/TripGo-Avito-/internal/worker/positions"
)

// App — собранный процесс. main запускает Server и по остановке сначала
// останавливает фон, потом вызывает Close.
type App struct {
	Server         *http.Server
	Logger         *slog.Logger
	once           sync.Once
	close          func(context.Context)
	stopBackground func(context.Context) error
}

func (a *App) Close(ctx context.Context) {
	if a.close == nil {
		return
	}
	a.once.Do(func() { a.close(ctx) })
}

// StopBackground останавливает сканер и даёт воркерам доработать очередь.
func (a *App) StopBackground(ctx context.Context) error {
	if a.stopBackground == nil {
		return nil
	}
	return a.stopBackground(ctx)
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
	points := position.New(pool, cfg.DatabaseQueryTimeout)
	keys := idempotency.New(pool, cfg.DatabaseQueryTimeout)
	tx := transaction.New(pool, cfg.DatabaseQueryTimeout, log)
	clock := systemClock{}
	meter := tel.Meter.Meter("trip-service")

	if err := telemetry.WatchActiveTrips(meter, log, trips.CountActive); err != nil {
		return nil, err
	}

	pushClient, err := push.New(cfg.PushHTTPURL, cfg.PushRequestTimeout, tel.Tracer.Tracer("trip-service"))
	if err != nil {
		return nil, err
	}
	poller, err := positions.New(positions.Config{
		Interval:    cfg.PositionCheckInterval,
		StaleAfter:  cfg.PositionStaleAfter,
		Workers:     cfg.PositionWorkers,
		QueueSize:   cfg.PositionQueueSize,
		BatchSize:   cfg.PositionScanBatchSize,
		PushTimeout: cfg.PushRequestTimeout,
	}, positions.Dependencies{
		Trips:   trips,
		Push:    pushClient,
		Metrics: tel.Positions,
		Log:     log,
		Tracer:  tel.Tracer.Tracer("trip-service"),
		Now:     clock.Now,
	})
	if err != nil {
		pushClient.Close()
		return nil, err
	}
	if err := telemetry.WatchPositionPool(meter, poller.QueueSize, poller.ActiveWorkers); err != nil {
		pushClient.Close()
		return nil, err
	}

	createTrip := createuc.New(tx, trips, history, keys, cfg.IdempotencyTTL, clock, systemIDs{}, log, tel.Trips)
	getTrip := getuc.New(trips)
	finishTrip := finishuc.New(tx, trips, history, clock, log, tel.Trips)
	addPosition := adduc.New(tx, points, clock, log, tel.Positions)
	listPositions := listuc.New(trips, points)

	handler, err := server.New(
		httpcreate.New(createTrip, log),
		httpget.New(getTrip, log),
		httpfinish.New(finishTrip, log),
		httpadd.New(addPosition, poller, log),
		httplist.New(listPositions, log),
		health.New(),
		ready.New(pool, cfg.DatabaseConnectTimeout, log),
		log,
		tel.Tracer,
		meter,
	)
	if err != nil {
		pushClient.Close()
		return nil, err
	}
	poller.Start(context.Background())

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
		stopBackground: poller.Shutdown,
		close: func(ctx context.Context) {
			pushClient.Close()
			if err := tel.Shutdown(ctx); err != nil {
				log.ErrorContext(ctx, "telemetry shutdown", "err", err)
			}
			pool.Close()
		},
	}, nil
}
