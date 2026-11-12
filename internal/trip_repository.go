package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const activeDriverConstraint = "trips_one_active_driver_idx"

var tripColumns = []string{
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
}

var sb = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

type TripRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *TripRepository) Insert(ctx context.Context, trip Trip) (Trip, error) {
	ctx, cancel := withTimeout(ctx, r.queryTimeout)
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
		Suffix("RETURNING " + strings.Join(tripColumns, ", ")).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build insert trip: %w", err)
	}

	stored, err := scanTrip(executor(ctx, r.pool).QueryRow(ctx, query, args...))
	if err != nil {
		if isActiveDriverConflict(err) {
			return Trip{}, ErrDriverBusy
		}
		return Trip{}, fmt.Errorf("insert trip: %w", err)
	}
	return stored, nil
}

// MarkCompleted переводит поездку в completed только если она ещё active.
// Ноль строк — это не ошибка SQL: поездки нет или её уже завершили.
func (r *TripRepository) MarkCompleted(ctx context.Context, id uuid.UUID, finishedAt time.Time) (Trip, bool, error) {
	ctx, cancel := withTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Update("trips").
		Set("status", statusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{"id": id, "status": statusActive}).
		Suffix("RETURNING " + strings.Join(tripColumns, ", ")).
		ToSql()
	if err != nil {
		return Trip{}, false, fmt.Errorf("build finish trip: %w", err)
	}

	stored, err := scanTrip(executor(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, false, nil
	}
	if err != nil {
		return Trip{}, false, fmt.Errorf("finish trip: %w", err)
	}
	return stored, true, nil
}

func (r *TripRepository) Get(ctx context.Context, id uuid.UUID) (Trip, error) {
	ctx, cancel := withTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Select(tripColumns...).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	stored, err := scanTrip(executor(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, ErrTripNotFound
	}
	if err != nil {
		return Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return stored, nil
}

func scanTrip(row pgx.Row) (Trip, error) {
	var trip Trip
	err := row.Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartLat,
		&trip.StartLon,
		&trip.EndLat,
		&trip.EndLon,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)
	if err != nil {
		return Trip{}, err
	}
	trip.StartedAt = trip.StartedAt.UTC()
	if trip.FinishedAt != nil {
		finishedAt := trip.FinishedAt.UTC()
		trip.FinishedAt = &finishedAt
	}
	return trip, nil
}

func isActiveDriverConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == activeDriverConstraint
}
