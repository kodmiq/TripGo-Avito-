package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Select(
		"id",
		"user_id",
		"driver_id",
		"start_latitude",
		"start_longitude",
		"end_latitude",
		"end_longitude",
		"price",
		"status",
		"started_at",
		"finished_at",
	).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	stored, err := scanTrip(r.query(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, errNoRows) {
		return model.Trip{}, domain.ErrTripNotFound
	}
	if err != nil {
		return model.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return stored, nil
}

// CountActive считает поездки в статусе active. Значение снимает gauge телеметрии.
func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Select("count(*)").
		From("trips").
		Where(squirrel.Eq{"status": string(domain.StatusActive)}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips: %w", err)
	}

	var count int64
	if err := r.query(ctx).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}
