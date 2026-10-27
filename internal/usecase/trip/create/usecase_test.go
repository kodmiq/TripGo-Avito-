package create_test

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
	"github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/create"
)

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

type stubIDs struct{ id uuid.UUID }

func (s stubIDs) New() uuid.UUID { return s.id }

type noopMetrics struct{}

func (noopMetrics) TripCreated(context.Context) {}

func runTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type harness struct {
	uc      *create.Usecase
	trips   *MocktripRepository
	history *MockhistoryRepository
	keys    *MockidempotencyRepository
	tx      *MocktxManager
	now     time.Time
	id      uuid.UUID
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctrl := gomock.NewController(t)
	h := &harness{
		trips:   NewMocktripRepository(ctrl),
		history: NewMockhistoryRepository(ctrl),
		keys:    NewMockidempotencyRepository(ctrl),
		tx:      NewMocktxManager(ctrl),
		now:     time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		id:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
	}
	h.uc = create.New(h.tx, h.trips, h.history, h.keys, 24*time.Hour, stubClock{now: h.now}, stubIDs{id: h.id}, slog.New(slog.DiscardHandler), noopMetrics{})
	return h
}

func validInput() model.CreateTrip {
	return model.CreateTrip{
		UserID:   uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"),
		DriverID: uuid.MustParse("6cb72c04-7650-45c9-a79b-bcdba0631e0c"),
		StartLat: 59.9398,
		StartLon: 30.3146,
		EndLat:   59.9290,
		EndLon:   30.3626,
		Price:    1450,
	}
}

func TestCreateSuccess(t *testing.T) {
	h := newHarness(t)
	in := validInput()
	saved := model.Trip{ID: h.id, UserID: in.UserID, DriverID: in.DriverID, Price: in.Price, Status: "active", StartedAt: h.now}

	h.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	h.trips.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, trip model.Trip) (model.Trip, error) {
		if trip.ID != h.id || trip.Status != string(domain.StatusActive) || trip.FinishedAt != nil || trip.Price != in.Price {
			t.Fatalf("saved draft = %+v", trip)
		}
		return saved, nil
	})
	h.history.EXPECT().Save(gomock.Any(), model.StatusChange{
		TripID:    h.id,
		ToStatus:  string(domain.StatusActive),
		Reason:    "trip created",
		ChangedAt: h.now,
	}).Return(nil)

	got, created, err := h.uc.Create(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if !created || got.ID != h.id || got.Status != "active" {
		t.Fatalf("Create() = %+v, created %v", got, created)
	}
}

func TestCreateDriverBusy(t *testing.T) {
	h := newHarness(t)
	h.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	h.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(model.Trip{}, domain.ErrDriverBusy)

	_, _, err := h.uc.Create(context.Background(), validInput(), nil)
	if !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("Create() = %v, want ErrDriverBusy", err)
	}
}

func TestCreateRejectsInvalidTripBeforeStorage(t *testing.T) {
	h := newHarness(t)
	in := validInput()
	in.Price = -1

	_, _, err := h.uc.Create(context.Background(), in, nil)
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Create() = %v, want validation", err)
	}
}

func TestCreateRepositoryError(t *testing.T) {
	h := newHarness(t)
	repoErr := errors.New("disk failed: secret-path")
	h.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	h.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(model.Trip{}, repoErr)

	_, _, err := h.uc.Create(context.Background(), validInput(), nil)
	if !errors.Is(err, repoErr) {
		t.Fatalf("Create() = %v, want %v", err, repoErr)
	}
}

func TestCreateReplaySameBody(t *testing.T) {
	h := newHarness(t)
	in := validInput()
	key := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	var storedHash string
	saved := model.Trip{ID: h.id, Status: "active", StartedAt: h.now}

	gomock.InOrder(
		h.keys.EXPECT().Get(gomock.Any(), key).Return(model.IdempotencyRecord{}, domain.ErrIdempotencyKeyNotFound),
		h.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx),
		h.keys.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, rec model.IdempotencyRecord) error {
			if rec.Key != key || rec.TripID != h.id || !rec.ExpiresAt.Equal(h.now.Add(24*time.Hour)) {
				t.Fatalf("idempotency record = %+v", rec)
			}
			storedHash = rec.RequestHash
			return nil
		}),
		h.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(saved, nil),
		h.history.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
	)
	if _, created, err := h.uc.Create(context.Background(), in, &key); err != nil || !created {
		t.Fatalf("first Create() created=%v err=%v", created, err)
	}

	h.keys.EXPECT().Get(gomock.Any(), key).Return(model.IdempotencyRecord{
		Key: key, RequestHash: storedHash, TripID: h.id, ExpiresAt: h.now.Add(time.Hour),
	}, nil)
	h.trips.EXPECT().Get(gomock.Any(), h.id).Return(saved, nil)

	got, created, err := h.uc.Create(context.Background(), in, &key)
	if err != nil {
		t.Fatalf("replay Create() = %v", err)
	}
	if created || got.ID != h.id {
		t.Fatalf("replay Create() = %+v, created %v", got, created)
	}
}

func TestCreateIdempotencyConflict(t *testing.T) {
	h := newHarness(t)
	key := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	h.keys.EXPECT().Get(gomock.Any(), key).Return(model.IdempotencyRecord{
		Key: key, RequestHash: "other-body", TripID: h.id, ExpiresAt: h.now.Add(time.Hour),
	}, nil)

	_, _, err := h.uc.Create(context.Background(), validInput(), &key)
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("Create() = %v, want ErrIdempotencyConflict", err)
	}
}

func TestCreateReplacesExpiredKey(t *testing.T) {
	h := newHarness(t)
	key := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	expiredAt := h.now.Add(-time.Minute)
	expired := model.IdempotencyRecord{Key: key, RequestHash: "old", TripID: h.id, ExpiresAt: expiredAt}
	saved := model.Trip{ID: h.id, Status: "active", StartedAt: h.now}

	gomock.InOrder(
		h.keys.EXPECT().Get(gomock.Any(), key).Return(expired, nil),
		h.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx),
		h.keys.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(domain.ErrIdempotencyKeyExists),
		h.keys.EXPECT().Get(gomock.Any(), key).Return(expired, nil),
		h.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx),
		h.keys.EXPECT().DeleteExact(gomock.Any(), key, expiredAt).Return(true, nil),
		h.keys.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil),
		h.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(saved, nil),
		h.history.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
	)

	got, created, err := h.uc.Create(context.Background(), validInput(), &key)
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if !created || got.ID != h.id {
		t.Fatalf("Create() = %+v, created %v", got, created)
	}
}

func TestCreateGivesUpWhenKeyDisappears(t *testing.T) {
	h := newHarness(t)
	key := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	h.keys.EXPECT().Get(gomock.Any(), key).Return(model.IdempotencyRecord{}, domain.ErrIdempotencyKeyNotFound).Times(4)
	h.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx).Times(2)
	h.keys.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(domain.ErrIdempotencyKeyExists).Times(2)

	_, _, err := h.uc.Create(context.Background(), validInput(), &key)
	if err == nil || errors.Is(err, domain.ErrIdempotencyKeyExists) {
		t.Fatalf("Create() = %v, want a terminal store error", err)
	}
}
