package listpositions

import (
	"context"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=listpositions_test

type lister interface {
	List(ctx context.Context, tripID uuid.UUID) ([]model.Position, error)
}
