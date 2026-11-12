package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
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
	ErrTripNotFound           = errors.New("trip not found")
	ErrTripCompleted          = errors.New("trip completed")
	ErrDriverBusy             = errors.New("driver busy")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
	ErrIdempotencyKeyNotFound = errors.New("idempotency key not found")
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
	keys    *IdempotencyRepository
	ttl     time.Duration
}

func NewTripService(tx TxManager, trips *TripRepository, history *TripStatusHistoryRepository, keys *IdempotencyRepository, ttl time.Duration) *TripService {
	return &TripService{tx: tx, trips: trips, history: history, keys: keys, ttl: ttl}
}

// Create пишет поездку и первую запись журнала в одной транзакции.
// id и started_at назначает сервис.
// idempotencyKey == nil — обычное создание, ответ 201.
// С ключом повтор того же тела возвращает прежнюю поездку и created=false (HTTP 200).
func (s *TripService) Create(ctx context.Context, in CreateTrip, idempotencyKey *uuid.UUID) (Trip, bool, error) {
	if idempotencyKey == nil {
		trip, err := s.insertTrip(ctx, in, nil, "", nil)
		if err != nil {
			return Trip{}, false, err
		}
		return trip, true, nil
	}
	trip, created, err := s.createIdempotent(ctx, in, *idempotencyKey)
	if err != nil {
		return Trip{}, false, err
	}
	return trip, created, nil
}

func (s *TripService) createIdempotent(ctx context.Context, in CreateTrip, key uuid.UUID) (Trip, bool, error) {
	hash := hashCreateTrip(in)
	for attempt := 0; attempt < 2; attempt++ {
		trip, found, err := s.lookupReplay(ctx, key, hash)
		if err != nil || found {
			return trip, false, err
		}

		stored, err := s.insertTrip(ctx, in, &key, hash, nil)
		if err == nil {
			return stored, true, nil
		}
		if !errors.Is(err, ErrIdempotencyKeyExists) {
			return Trip{}, false, err
		}

		rec, getErr := s.keys.Get(ctx, key)
		if errors.Is(getErr, ErrIdempotencyKeyNotFound) {
			continue
		}
		if getErr != nil {
			return Trip{}, false, getErr
		}
		if rec.ExpiresAt.After(nowUTC()) {
			trip, err = s.replayRecord(ctx, rec, hash)
			return trip, false, err
		}

		stored, err = s.insertTrip(ctx, in, &key, hash, &rec.ExpiresAt)
		if err == nil {
			return stored, true, nil
		}
		if errors.Is(err, ErrIdempotencyKeyExists) {
			trip, found, replayErr := s.lookupReplay(ctx, key, hash)
			if replayErr != nil || found {
				return trip, false, replayErr
			}
			continue
		}
		return Trip{}, false, err
	}
	return Trip{}, false, fmt.Errorf("create trip: idempotency key %s was not stored", key)
}

func (s *TripService) lookupReplay(ctx context.Context, key uuid.UUID, hash string) (Trip, bool, error) {
	rec, err := s.keys.Get(ctx, key)
	if errors.Is(err, ErrIdempotencyKeyNotFound) {
		return Trip{}, false, nil
	}
	if err != nil {
		return Trip{}, false, err
	}
	if !rec.ExpiresAt.After(nowUTC()) {
		return Trip{}, false, nil
	}
	trip, err := s.replayRecord(ctx, rec, hash)
	if err != nil {
		return Trip{}, false, err
	}
	return trip, true, nil
}

func (s *TripService) replayRecord(ctx context.Context, rec IdempotencyRecord, hash string) (Trip, error) {
	if rec.RequestHash != hash {
		return Trip{}, ErrIdempotencyConflict
	}
	return s.trips.Get(ctx, rec.TripID)
}

// insertTrip создаёт поездку. key заменяет строку с expiresAt, если он не nil:
// так повторно используется уже истёкший ключ, не задевая чужую новую запись.
func (s *TripService) insertTrip(ctx context.Context, in CreateTrip, key *uuid.UUID, hash string, replaceExpiresAt *time.Time) (Trip, error) {
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
		if key != nil && replaceExpiresAt != nil {
			deleted, err := s.keys.DeleteExact(ctx, *key, *replaceExpiresAt)
			if err != nil {
				return err
			}
			if !deleted {
				return ErrIdempotencyKeyExists
			}
		}
		if key != nil {
			now := nowUTC()
			if err := s.keys.Insert(ctx, IdempotencyRecord{
				Key:         *key,
				RequestHash: hash,
				TripID:      draft.ID,
				CreatedAt:   now,
				ExpiresAt:   now.Add(s.ttl),
			}); err != nil {
				return err
			}
		}
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

func hashCreateTrip(in CreateTrip) string {
	canonical := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d",
		in.UserID,
		in.DriverID,
		strconv.FormatFloat(in.StartLat, 'f', -1, 64),
		strconv.FormatFloat(in.StartLon, 'f', -1, 64),
		strconv.FormatFloat(in.EndLat, 'f', -1, 64),
		strconv.FormatFloat(in.EndLon, 'f', -1, 64),
		in.Price,
	)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
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
