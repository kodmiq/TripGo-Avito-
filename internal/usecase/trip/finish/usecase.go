package finish

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

const reasonFinished = "trip finished"

// Usecase завершает активную поездку и пишет переход в журнал.
type Usecase struct {
	tx      txManager
	trips   tripRepository
	history historyRepository
	clock   clock
	log     *slog.Logger
	metrics metrics
}

func New(tx txManager, trips tripRepository, history historyRepository, clock clock, log *slog.Logger, metrics metrics) *Usecase {
	return &Usecase{tx: tx, trips: trips, history: history, clock: clock, log: log, metrics: metrics}
}

// Finish атомарно переводит active → completed.
// Условие status = active стоит в самом UPDATE, поэтому второй параллельный
// вызов не перезаписывает finished_at. Правило перехода проверяет домен.
func (u *Usecase) Finish(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	var finished model.Trip
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		current, err := u.trips.Get(ctx, id)
		if err != nil {
			return err
		}
		entity := toDomain(current)
		finishedAt := u.clock.Now()
		if err := entity.Finish(finishedAt); err != nil {
			return err
		}

		row, ok, err := u.trips.MarkCompleted(ctx, id, finishedAt)
		if err != nil {
			return err
		}
		if !ok {
			// Между чтением и UPDATE статус успели сменить. Повторное чтение
			// отличает «уже завершена» от неожиданного состояния.
			current, err = u.trips.Get(ctx, id)
			if err != nil {
				return err
			}
			raced := toDomain(current)
			if err := raced.Finish(finishedAt); err != nil {
				return err
			}
			return fmt.Errorf("finish trip %s: status %q was not updated", id, current.Status)
		}

		from := string(domain.StatusActive)
		changedAt := finishedAt
		if row.FinishedAt != nil {
			changedAt = *row.FinishedAt
		}
		if err := u.history.Save(ctx, model.StatusChange{
			TripID:     id,
			FromStatus: &from,
			ToStatus:   string(domain.StatusCompleted),
			Reason:     reasonFinished,
			ChangedAt:  changedAt,
		}); err != nil {
			return err
		}
		finished = row
		return nil
	})
	if err != nil {
		return model.Trip{}, err
	}
	u.metrics.TripCompleted(ctx)
	u.log.InfoContext(ctx, "trip completed", "trip_id", finished.ID.String(), "status", finished.Status)
	return finished, nil
}

func toDomain(m model.Trip) domain.Trip {
	return domain.Trip{
		ID:         m.ID,
		UserID:     m.UserID,
		DriverID:   m.DriverID,
		StartLat:   m.StartLat,
		StartLon:   m.StartLon,
		EndLat:     m.EndLat,
		EndLon:     m.EndLon,
		Price:      m.Price,
		Status:     domain.Status(m.Status),
		StartedAt:  m.StartedAt,
		FinishedAt: m.FinishedAt,
	}
}
