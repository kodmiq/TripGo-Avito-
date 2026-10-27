package trip

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

var errNoRows = pgx.ErrNoRows

func scanTrip(row row) (model.Trip, error) {
	var trip model.Trip
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
		return model.Trip{}, err
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
