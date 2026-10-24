package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/kodmiq/TripGo-Avito-/internal"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
)

//go:generate go tool oapi-codegen -generate types,chi-server -package api -include-operation-ids createTrip,getTrip,finishTrip,health,ready -o ../../internal/generated/api.gen.go ../../contracts/openapi/trip-service.openapi.yaml

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "trip-service")
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error("service stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadEnvFiles(); err != nil {
		return err
	}
	cfg, err := app.ParseEnv()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).With("service", "trip-service"))

	poolCtx, cancelPool := context.WithTimeout(context.Background(), cfg.DatabaseConnectTimeout)
	pool, err := app.NewPool(poolCtx, cfg)
	cancelPool()
	if err != nil {
		return err
	}
	defer pool.Close()

	trips := app.NewTripRepository(pool, cfg.DatabaseQueryTimeout)
	history := app.NewTripStatusHistoryRepository(pool, cfg.DatabaseQueryTimeout)
	keys := app.NewIdempotencyRepository(pool, cfg.DatabaseQueryTimeout)
	service := app.NewTripService(app.NewTxManager(pool, cfg.DatabaseQueryTimeout), trips, history, keys, cfg.IdempotencyTTL)
	handler := app.NewHandler(service, pool.Ping, cfg.DatabaseConnectTimeout)

	router := chi.NewRouter()
	router.Use(app.RecoverMiddleware)
	api.HandlerWithOptions(handler, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: handler.InvalidParam,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server started", "addr", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-signalCtx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown budget of %s exceeded: %w", cfg.ShutdownTimeout, err)
	}
	slog.Info("shutdown complete")
	return nil
}

func loadEnvFiles() error {
	if _, err := os.Stat(".env.example"); err == nil {
		if err := godotenv.Load(".env.example"); err != nil {
			return err
		}
	}
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Overload(".env"); err != nil {
			return err
		}
	}
	return nil
}
