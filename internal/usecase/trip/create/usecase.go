package create

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

const (
	reasonCreated = "trip created"
)

// Usecase создаёт поездку и первую запись журнала в одной транзакции.
type Usecase struct {
	tx      txManager
	trips   tripRepository
	history historyRepository
	keys    idempotencyRepository
	ttl     time.Duration
	clock   clock
	ids     idGenerator
	log     *slog.Logger
}

func New(
	tx txManager,
	trips tripRepository,
	history historyRepository,
	keys idempotencyRepository,
	ttl time.Duration,
	clock clock,
	ids idGenerator,
	log *slog.Logger,
) *Usecase {
	return &Usecase{
		tx: tx, trips: trips, history: history, keys: keys,
		ttl: ttl, clock: clock, ids: ids, log: log,
	}
}

// Create пишет поездку. idempotencyKey == nil — обычное создание, created=true.
// С ключом повтор того же тела возвращает прежнюю поездку и created=false.
func (u *Usecase) Create(ctx context.Context, in model.CreateTrip, idempotencyKey *uuid.UUID) (model.Trip, bool, error) {
	if err := domain.ValidateCreate(in.UserID, in.DriverID, in.StartLat, in.StartLon, in.EndLat, in.EndLon, in.Price); err != nil {
		return model.Trip{}, false, err
	}
	if idempotencyKey == nil {
		trip, err := u.insertTrip(ctx, in, nil, "", nil)
		if err != nil {
			return model.Trip{}, false, err
		}
		return trip, true, nil
	}
	return u.createIdempotent(ctx, in, *idempotencyKey)
}

func (u *Usecase) createIdempotent(ctx context.Context, in model.CreateTrip, key uuid.UUID) (model.Trip, bool, error) {
	hash := hashCreateTrip(in)
	for range 2 {
		trip, found, err := u.lookupReplay(ctx, key, hash)
		if err != nil || found {
			return trip, false, err
		}

		stored, err := u.insertTrip(ctx, in, &key, hash, nil)
		if err == nil {
			return stored, true, nil
		}
		if !errors.Is(err, domain.ErrIdempotencyKeyExists) {
			return model.Trip{}, false, err
		}

		rec, getErr := u.keys.Get(ctx, key)
		if errors.Is(getErr, domain.ErrIdempotencyKeyNotFound) {
			continue
		}
		if getErr != nil {
			return model.Trip{}, false, getErr
		}
		if rec.ExpiresAt.After(u.clock.Now()) {
			trip, err = u.replayRecord(ctx, rec, hash)
			return trip, false, err
		}

		stored, err = u.insertTrip(ctx, in, &key, hash, &rec.ExpiresAt)
		if err == nil {
			return stored, true, nil
		}
		if errors.Is(err, domain.ErrIdempotencyKeyExists) {
			trip, found, replayErr := u.lookupReplay(ctx, key, hash)
			if replayErr != nil || found {
				return trip, false, replayErr
			}
			continue
		}
		return model.Trip{}, false, err
	}
	return model.Trip{}, false, fmt.Errorf("create trip: idempotency key %s was not stored", key)
}

func (u *Usecase) lookupReplay(ctx context.Context, key uuid.UUID, hash string) (model.Trip, bool, error) {
	rec, err := u.keys.Get(ctx, key)
	if errors.Is(err, domain.ErrIdempotencyKeyNotFound) {
		return model.Trip{}, false, nil
	}
	if err != nil {
		return model.Trip{}, false, err
	}
	if !rec.ExpiresAt.After(u.clock.Now()) {
		return model.Trip{}, false, nil
	}
	trip, err := u.replayRecord(ctx, rec, hash)
	if err != nil {
		return model.Trip{}, false, err
	}
	return trip, true, nil
}

func (u *Usecase) replayRecord(ctx context.Context, rec model.IdempotencyRecord, hash string) (model.Trip, error) {
	if rec.RequestHash != hash {
		return model.Trip{}, domain.ErrIdempotencyConflict
	}
	return u.trips.Get(ctx, rec.TripID)
}

// insertTrip создаёт поездку. key заменяет строку с expiresAt, если он не nil:
// так повторно используется уже истёкший ключ, не задевая чужую новую запись.
func (u *Usecase) insertTrip(ctx context.Context, in model.CreateTrip, key *uuid.UUID, hash string, replaceExpiresAt *time.Time) (model.Trip, error) {
	draftTrip, err := domain.NewActive(
		u.ids.New(),
		in.UserID,
		in.DriverID,
		in.StartLat,
		in.StartLon,
		in.EndLat,
		in.EndLon,
		in.Price,
		u.clock.Now(),
	)
	if err != nil {
		return model.Trip{}, err
	}
	draft := toModel(draftTrip)

	var stored model.Trip
	err = u.tx.Do(ctx, func(ctx context.Context) error {
		if key != nil && replaceExpiresAt != nil {
			deleted, err := u.keys.DeleteExact(ctx, *key, *replaceExpiresAt)
			if err != nil {
				return err
			}
			if !deleted {
				return domain.ErrIdempotencyKeyExists
			}
		}
		if key != nil {
			now := u.clock.Now()
			if err := u.keys.Insert(ctx, model.IdempotencyRecord{
				Key:         *key,
				RequestHash: hash,
				TripID:      draft.ID,
				CreatedAt:   now,
				ExpiresAt:   now.Add(u.ttl),
			}); err != nil {
				return err
			}
		}
		row, err := u.trips.Save(ctx, draft)
		if err != nil {
			return err
		}
		if err := u.history.Save(ctx, model.StatusChange{
			TripID:    row.ID,
			ToStatus:  string(domain.StatusActive),
			Reason:    reasonCreated,
			ChangedAt: row.StartedAt,
		}); err != nil {
			return err
		}
		stored = row
		return nil
	})
	if err != nil {
		return model.Trip{}, err
	}
	u.log.InfoContext(ctx, "trip created", "trip_id", stored.ID.String(), "status", stored.Status)
	return stored, nil
}

func toModel(t domain.Trip) model.Trip {
	return model.Trip{
		ID:         t.ID,
		UserID:     t.UserID,
		DriverID:   t.DriverID,
		StartLat:   t.StartLat,
		StartLon:   t.StartLon,
		EndLat:     t.EndLat,
		EndLon:     t.EndLon,
		Price:      t.Price,
		Status:     string(t.Status),
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}
