package get

import (
	"context"

	"github.com/google/uuid"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// Usecase читает одну поездку.
type Usecase struct {
	trips tripRepository
}

func New(trips tripRepository) *Usecase {
	return &Usecase{trips: trips}
}

func (u *Usecase) Get(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	return u.trips.Get(ctx, id)
}
