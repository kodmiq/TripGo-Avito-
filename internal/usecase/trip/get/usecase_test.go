package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
	"github.com/kodmiq/TripGo-Avito-/internal/usecase/trip/get"
)

func TestGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	uc := get.New(repo)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	stored := model.Trip{ID: id, Status: "active"}

	repo.EXPECT().Get(gomock.Any(), id).Return(stored, nil)
	got, err := uc.Get(context.Background(), id)
	if err != nil || got.ID != id {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
}

func TestGetNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	uc := get.New(repo)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	repo.EXPECT().Get(gomock.Any(), id).Return(model.Trip{}, domain.ErrTripNotFound)
	_, err := uc.Get(context.Background(), id)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("Get() = %v, want ErrTripNotFound", err)
	}
}
