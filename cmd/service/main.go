package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kodmiq/TripGo-Avito-/internal/config"
	"github.com/kodmiq/TripGo-Avito-/internal/container"
)

//go:generate go tool oapi-codegen -generate types,chi-server -package api -include-operation-ids createTrip,getTrip,finishTrip,createTripPosition,listTripPositions,health,ready -o ../../internal/generated/api.gen.go ../../contracts/openapi/trip-service.openapi.yaml

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "service stopped: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := config.LoadDotEnv(); err != nil {
		return err
	}
	cfg, err := config.Parse()
	if err != nil {
		return err
	}

	app, err := container.New(context.Background(), cfg)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		app.Logger.Info("http server started", "addr", cfg.HTTPAddr)
		errCh <- app.Server.ListenAndServe()
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var serveErr error
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	case <-signalCtx.Done():
		app.Logger.Info("shutdown signal received")
	}

	// Один бюджет на всю остановку, порядок фиксирован:
	// 1. HTTP перестаёт принимать новое и дожидается текущих запросов.
	// 2–3. StopBackground гасит сканер, затем воркеры дорабатывают очередь.
	// 4. Close закрывает клиент Push Service, сбрасывает телеметрию и пул БД.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := app.Server.Shutdown(shutdownCtx); err != nil {
		app.Logger.Error("shutdown budget exceeded", "timeout", cfg.ShutdownTimeout.String(), "err", err)
		if serveErr == nil {
			serveErr = fmt.Errorf("shutdown budget of %s exceeded: %w", cfg.ShutdownTimeout, err)
		}
	}
	if err := app.StopBackground(shutdownCtx); err != nil {
		app.Logger.Error("background shutdown", "err", err)
		if serveErr == nil {
			serveErr = err
		}
	}
	app.Close(shutdownCtx)
	if serveErr != nil {
		return serveErr
	}
	app.Logger.Info("shutdown complete")
	return nil
}
