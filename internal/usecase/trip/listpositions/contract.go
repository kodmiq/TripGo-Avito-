package listpositions

import (
	"context"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=listpositions_test

type (
	tripRepository interface {
		Get(ctx context.Context, id uuid.UUID) (model.Trip, error)
	}

	positionRepository interface {
		List(ctx context.Context, tripID uuid.UUID) ([]model.Position, error)
	}
)
