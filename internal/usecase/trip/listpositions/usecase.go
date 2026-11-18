package listpositions

import (
	"context"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// Usecase отдаёт маршрут поездки в порядке записи точек.
type Usecase struct {
	trips     tripRepository
	positions positionRepository
}

func New(trips tripRepository, positions positionRepository) *Usecase {
	return &Usecase{trips: trips, positions: positions}
}

// List возвращает все точки. Поездки нет — та же ошибка, что у чтения поездки.
func (u *Usecase) List(ctx context.Context, tripID uuid.UUID) ([]model.Position, error) {
	if _, err := u.trips.Get(ctx, tripID); err != nil {
		return nil, err
	}
	positions, err := u.positions.List(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if positions == nil {
		return []model.Position{}, nil
	}
	return positions, nil
}
