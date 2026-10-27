package finish

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=finish_test

type (
	tripRepository interface {
		Get(ctx context.Context, id uuid.UUID) (model.Trip, error)
		MarkCompleted(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, bool, error)
	}

	historyRepository interface {
		Save(ctx context.Context, change model.StatusChange) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	clock interface {
		Now() time.Time
	}
)
