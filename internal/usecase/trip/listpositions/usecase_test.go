package listpositions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
	"github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/listpositions"
)

func TestListRequiresTrip(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	positions := NewMockpositionRepository(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc := listpositions.New(trips, positions)

	trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{}, domain.ErrTripNotFound)

	_, err := uc.List(context.Background(), id)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("List() = %v", err)
	}
}

func TestListReturnsRepositoryOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	positions := NewMockpositionRepository(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc := listpositions.New(trips, positions)
	const wantID int64 = 1
	want := []model.Position{{
		ID: wantID, TripID: id, Latitude: 1, Longitude: 2,
		RecordedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
	}}

	trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{ID: id}, nil)
	positions.EXPECT().List(gomock.Any(), id).Return(want, nil)

	got, err := uc.List(context.Background(), id)
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List() = %+v", got)
	}
	if got[0].ID != wantID {
		t.Fatalf("List() = %+v", got)
	}
}

func TestListEmptyIsNotNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	positions := NewMockpositionRepository(ctrl)
	id := uuid.New()
	uc := listpositions.New(trips, positions)

	trips.EXPECT().Get(gomock.Any(), id).Return(model.Trip{ID: id}, nil)
	positions.EXPECT().List(gomock.Any(), id).Return(nil, nil)

	got, err := uc.List(context.Background(), id)
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("List() = %#v", got)
	}
}
