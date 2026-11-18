package addposition

import (
	"context"
	"time"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=addposition_test

type (
	positionRepository interface {
		Save(ctx context.Context, in model.NewPosition, updatedAt time.Time) (model.Position, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	clock interface {
		Now() time.Time
	}

	metrics interface {
		PositionSaved(ctx context.Context)
	}
)
