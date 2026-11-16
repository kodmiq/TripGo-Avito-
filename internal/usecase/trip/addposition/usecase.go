package addposition

import (
	"context"
	"log/slog"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// Usecase сохраняет координату активной поездки.
type Usecase struct {
	tx        txManager
	positions positionRepository
	clock     clock
	log       *slog.Logger
	metrics   metrics
}

func New(tx txManager, positions positionRepository, clock clock, log *slog.Logger, metrics metrics) *Usecase {
	return &Usecase{tx: tx, positions: positions, clock: clock, log: log, metrics: metrics}
}

// Add проверяет координату и в одной транзакции пишет точку и last_position_at.
func (u *Usecase) Add(ctx context.Context, in model.NewPosition) (model.Position, error) {
	if err := domain.ValidatePosition(in.Latitude, in.Longitude, in.RecordedAt, u.clock.Now()); err != nil {
		return model.Position{}, err
	}

	var stored model.Position
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		row, err := u.positions.Save(ctx, in, u.clock.Now())
		if err != nil {
			return err
		}
		stored = row
		return nil
	})
	if err != nil {
		return model.Position{}, err
	}

	u.metrics.PositionSaved(ctx)
	u.log.InfoContext(ctx, "position saved", "trip_id", stored.TripID.String(), "position_id", stored.ID)
	return stored, nil
}
