package position

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// Save пишет точку и сдвигает last_position_at только вперёд.
// Обе записи должны идти в одной транзакции: вызывающий открывает её через TxManager.
// Сравнение времени стоит в UPDATE, поэтому опоздавшая координата не состаривает поездку.
func (r *Repository) Save(ctx context.Context, in model.NewPosition, updatedAt time.Time) (model.Position, error) {
	status, err := r.lockStatus(ctx, in.TripID)
	if err != nil {
		return model.Position{}, err
	}
	if status != string(domain.StatusActive) {
		return model.Position{}, domain.ErrTripCompleted
	}

	stored, err := r.insert(ctx, in)
	if err != nil {
		return model.Position{}, err
	}
	if err := r.advanceLastPosition(ctx, in, updatedAt); err != nil {
		return model.Position{}, err
	}
	return stored, nil
}

func (r *Repository) lockStatus(ctx context.Context, id uuid.UUID) (string, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Select("status").
		From("trips").
		Where(squirrel.Eq{"id": id}).
		Suffix("FOR UPDATE").
		ToSql()
	if err != nil {
		return "", fmt.Errorf("build lock trip: %w", err)
	}

	var status string
	err = r.query(ctx).QueryRow(ctx, query, args...).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrTripNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock trip: %w", err)
	}
	return status, nil
}

func (r *Repository) insert(ctx context.Context, in model.NewPosition) (model.Position, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(in.TripID, in.Latitude, in.Longitude, in.RecordedAt).
		Suffix("RETURNING id, trip_id, latitude, longitude, recorded_at").
		ToSql()
	if err != nil {
		return model.Position{}, fmt.Errorf("build insert position: %w", err)
	}

	stored, err := scanPosition(r.query(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		return model.Position{}, fmt.Errorf("insert position: %w", err)
	}
	return stored, nil
}

func (r *Repository) advanceLastPosition(ctx context.Context, in model.NewPosition, updatedAt time.Time) error {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Update("trips").
		Set("last_position_at", in.RecordedAt).
		Set("updated_at", updatedAt).
		Where(squirrel.Eq{"id": in.TripID}).
		Where(squirrel.Expr("(last_position_at IS NULL OR last_position_at < ?)", in.RecordedAt)).
		ToSql()
	if err != nil {
		return fmt.Errorf("build advance last position: %w", err)
	}
	if _, err := r.query(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("advance last position: %w", err)
	}
	return nil
}
