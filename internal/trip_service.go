package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	statusActive    = "active"
	statusCompleted = "completed"

	reasonCreated  = "trip created"
	reasonFinished = "trip finished"
)

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip completed")
	ErrDriverBusy    = errors.New("driver busy")
)

type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartLat   float64
	StartLon   float64
	EndLat     float64
	EndLon     float64
	Price      int64
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

type CreateTrip struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	StartLat float64
	StartLon float64
	EndLat   float64
	EndLon   float64
	Price    int64
}

type TripService struct {
	tx      TxManager
	trips   *TripRepository
	history *TripStatusHistoryRepository
}

func NewTripService(tx TxManager, trips *TripRepository, history *TripStatusHistoryRepository) *TripService {
	return &TripService{tx: tx, trips: trips, history: history}
}

// Create пишет поездку и первую запись журнала в одной транзакции.
// id и started_at назначает сервис.
func (s *TripService) Create(ctx context.Context, in CreateTrip) (Trip, error) {
	draft := Trip{
		ID:        uuid.New(),
		UserID:    in.UserID,
		DriverID:  in.DriverID,
		StartLat:  in.StartLat,
		StartLon:  in.StartLon,
		EndLat:    in.EndLat,
		EndLon:    in.EndLon,
		Price:     in.Price,
		Status:    statusActive,
		StartedAt: nowUTC(),
	}

	var stored Trip
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		row, err := s.trips.Insert(ctx, draft)
		if err != nil {
			return err
		}
		if err := s.history.Insert(ctx, StatusChange{
			TripID:    row.ID,
			ToStatus:  statusActive,
			Reason:    reasonCreated,
			ChangedAt: row.StartedAt,
		}); err != nil {
			return err
		}
		stored = row
		return nil
	})
	if err != nil {
		return Trip{}, err
	}
	return stored, nil
}

func (s *TripService) Get(ctx context.Context, id uuid.UUID) (Trip, error) {
	return s.trips.Get(ctx, id)
}

// Finish атомарно переводит active → completed.
// Условие status = active стоит в самом UPDATE, поэтому второй параллельный
// вызов не перезаписывает finished_at.
func (s *TripService) Finish(ctx context.Context, id uuid.UUID) (Trip, error) {
	var finished Trip
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		finishedAt := nowUTC()
		row, ok, err := s.trips.MarkCompleted(ctx, id, finishedAt)
		if err != nil {
			return err
		}
		if !ok {
			current, err := s.trips.Get(ctx, id)
			if err != nil {
				return err
			}
			if current.Status == statusCompleted {
				return ErrTripCompleted
			}
			return fmt.Errorf("finish trip %s: status %q was not updated", id, current.Status)
		}

		from := statusActive
		changedAt := finishedAt
		if row.FinishedAt != nil {
			changedAt = *row.FinishedAt
		}
		if err := s.history.Insert(ctx, StatusChange{
			TripID:     id,
			FromStatus: &from,
			ToStatus:   statusCompleted,
			Reason:     reasonFinished,
			ChangedAt:  changedAt,
		}); err != nil {
			return err
		}
		finished = row
		return nil
	})
	if err != nil {
		return Trip{}, err
	}
	return finished, nil
}

func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
