package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create_test

type creator interface {
	Create(ctx context.Context, in model.CreateTrip, idempotencyKey *uuid.UUID) (model.Trip, bool, error)
}
