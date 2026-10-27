package finish_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
	"github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/finish"
	"go.uber.org/mock/gomock"
)

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

type noopMetrics struct{}

func (noopMetrics) TripCompleted(context.Context) {}

func runTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestFinishActiveTrip(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	history := NewMockhistoryRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc := finish.New(tx, trips, history, stubClock{now: now}, slog.New(slog.DiscardHandler), noopMetrics{})

	active := model.Trip{ID: id, Status: string(domain.StatusActive), StartedAt: now.Add(-time.Hour)}
	finishedAt := now
	completed := active
	completed.Status = string(domain.StatusCompleted)
	completed.FinishedAt = &finishedAt

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	trips.EXPECT().Get(gomock.Any(), id).Return(active, nil)
	trips.EXPECT().MarkCompleted(gomock.Any(), id, now).Return(completed, true, nil)
	history.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, change model.StatusChange) error {
		if change.TripID != id || change.FromStatus == nil || *change.FromStatus != string(domain.StatusActive) || change.ToStatus != string(domain.StatusCompleted) {
			t.Fatalf("history = %+v", change)
		}
		if !change.ChangedAt.Equal(now) || change.Reason != "trip finished" {
			t.Fatalf("history time/reason = %+v", change)
		}
		return nil
	})

	got, err := uc.Finish(context.Background(), id)
	if err != nil {
		t.Fatalf("Finish() = %v", err)
	}
	if got.Status != string(domain.StatusCompleted) || got.FinishedAt == nil || !got.FinishedAt.Equal(now) {
		t.Fatalf("Finish() = %+v", got)
	}
}

func TestFinishCompletedTrip(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	history := NewMockhistoryRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	finishedAt := now.Add(-time.Hour)
	uc := finish.New(tx, trips, history, stubClock{now: now}, slog.New(slog.DiscardHandler), noopMetrics{})

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{
		ID: id, Status: string(domain.StatusCompleted), FinishedAt: &finishedAt,
	}, nil)

	_, err := uc.Finish(context.Background(), id)
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("Finish() = %v, want ErrTripCompleted", err)
	}
}

func TestFinishNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc := finish.New(tx, trips, NewMockhistoryRepository(ctrl), stubClock{now: time.Now().UTC()}, slog.New(slog.DiscardHandler), noopMetrics{})

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{}, domain.ErrTripNotFound)

	_, err := uc.Finish(context.Background(), id)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("Finish() = %v, want ErrTripNotFound", err)
	}
}

func TestFinishLostRace(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	finishedAt := now
	uc := finish.New(tx, trips, NewMockhistoryRepository(ctrl), stubClock{now: now}, slog.New(slog.DiscardHandler), noopMetrics{})

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	gomock.InOrder(
		trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{ID: id, Status: string(domain.StatusActive)}, nil),
		trips.EXPECT().MarkCompleted(gomock.Any(), id, now).Return(model.Trip{}, false, nil),
		trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{ID: id, Status: string(domain.StatusCompleted), FinishedAt: &finishedAt}, nil),
	)

	_, err := uc.Finish(context.Background(), id)
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("Finish() = %v, want ErrTripCompleted", err)
	}
}

func TestFinishRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repoErr := errors.New("update failed: secret")
	uc := finish.New(tx, trips, NewMockhistoryRepository(ctrl), stubClock{now: now}, slog.New(slog.DiscardHandler), noopMetrics{})

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{ID: id, Status: string(domain.StatusActive)}, nil)
	trips.EXPECT().MarkCompleted(gomock.Any(), id, now).Return(model.Trip{}, false, repoErr)

	_, err := uc.Finish(context.Background(), id)
	if !errors.Is(err, repoErr) {
		t.Fatalf("Finish() = %v, want %v", err, repoErr)
	}
}
