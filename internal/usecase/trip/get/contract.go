package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=get_test

type tripRepository interface {
	Get(ctx context.Context, id uuid.UUID) (model.Trip, error)
}
