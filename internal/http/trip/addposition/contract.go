package addposition

import (
	"context"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=addposition_test

type (
	adder interface {
		Add(ctx context.Context, in model.NewPosition) (model.Position, error)
	}

	// canceller убирает поездку из очереди опроса, когда координата уже пришла.
	canceller interface {
		Cancel(ctx context.Context, tripID uuid.UUID)
	}
)
