package addposition_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
	"github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/addposition"
)

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

type countingMetrics struct{ n int }

func (m *countingMetrics) PositionSaved(context.Context) { m.n++ }

func runTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestAddSavesInsideTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	positions := NewMockpositionRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	metrics := &countingMetrics{}
	uc := addposition.New(tx, positions, stubClock{now: now}, slog.New(slog.DiscardHandler), metrics)

	in := model.NewPosition{
		TripID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Latitude:   59.93,
		Longitude:  30.33,
		RecordedAt: now.Add(-time.Second),
	}
	stored := model.Position{ID: 7, TripID: in.TripID, Latitude: in.Latitude, Longitude: in.Longitude, RecordedAt: in.RecordedAt}

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	positions.EXPECT().Save(gomock.Any(), in, now).Return(stored, nil)

	got, err := uc.Add(context.Background(), in)
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}
	if got != stored {
		t.Fatalf("Add() = %+v", got)
	}
	if metrics.n != 1 {
		t.Fatalf("saved metric = %d", metrics.n)
	}
}

func TestAddRejectsFutureAndSkipsMetric(t *testing.T) {
	ctrl := gomock.NewController(t)
	positions := NewMockpositionRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	metrics := &countingMetrics{}
	uc := addposition.New(tx, positions, stubClock{now: now}, slog.New(slog.DiscardHandler), metrics)

	_, err := uc.Add(context.Background(), model.NewPosition{
		TripID:     uuid.New(),
		Latitude:   1,
		Longitude:  2,
		RecordedAt: now.Add(2 * time.Minute),
	})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Add() = %v, want validation", err)
	}
	if metrics.n != 0 {
		t.Fatalf("saved metric = %d", metrics.n)
	}
}

func TestAddReturnsRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	positions := NewMockpositionRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	metrics := &countingMetrics{}
	uc := addposition.New(tx, positions, stubClock{now: now}, slog.New(slog.DiscardHandler), metrics)
	in := model.NewPosition{TripID: uuid.New(), Latitude: 1, Longitude: 2, RecordedAt: now}

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	positions.EXPECT().Save(gomock.Any(), in, now).Return(model.Position{}, domain.ErrTripCompleted)

	_, err := uc.Add(context.Background(), in)
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("Add() = %v", err)
	}
	if metrics.n != 0 {
		t.Fatalf("saved metric = %d", metrics.n)
	}
}
