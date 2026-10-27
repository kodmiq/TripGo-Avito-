package create

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create_test

type (
	tripRepository interface {
		Save(ctx context.Context, trip model.Trip) (model.Trip, error)
		Get(ctx context.Context, id uuid.UUID) (model.Trip, error)
	}

	historyRepository interface {
		Save(ctx context.Context, change model.StatusChange) error
	}

	idempotencyRepository interface {
		Insert(ctx context.Context, rec model.IdempotencyRecord) error
		Get(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error)
		DeleteExact(ctx context.Context, key uuid.UUID, expiresAt time.Time) (bool, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	clock interface {
		Now() time.Time
	}

	idGenerator interface {
		New() uuid.UUID
	}

	metrics interface {
		TripCreated(ctx context.Context)
	}
)
