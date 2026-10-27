package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

var sb = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

func (r *Repository) Save(ctx context.Context, trip model.Trip) (model.Trip, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Insert("trips").
		Columns(
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
		Values(
			trip.ID,
			trip.UserID,
			trip.DriverID,
			trip.StartLat,
			trip.StartLon,
			trip.EndLat,
			trip.EndLon,
			trip.Price,
			trip.Status,
			trip.StartedAt,
			trip.FinishedAt,
		).
		Suffix("RETURNING " + tripReturning).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build insert trip: %w", err)
	}

	stored, err := scanTrip(r.query(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		if isActiveDriverConflict(err) {
			return model.Trip{}, domain.ErrDriverBusy
		}
		return model.Trip{}, fmt.Errorf("insert trip: %w", err)
	}
	return stored, nil
}

// MarkCompleted переводит поездку в completed только если она ещё active.
// Ноль строк — это не ошибка SQL: поездки нет или её уже завершили.
func (r *Repository) MarkCompleted(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, bool, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Update("trips").
		Set("status", string(domain.StatusCompleted)).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{"id": id, "status": string(domain.StatusActive)}).
		Suffix("RETURNING " + tripReturning).
		ToSql()
	if err != nil {
		return model.Trip{}, false, fmt.Errorf("build finish trip: %w", err)
	}

	stored, err := scanTrip(r.query(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, errNoRows) {
		return model.Trip{}, false, nil
	}
	if err != nil {
		return model.Trip{}, false, fmt.Errorf("finish trip: %w", err)
	}
	return stored, true, nil
}
