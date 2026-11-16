package position

import "github.com/kodmiq/TripGo-Avito-/internal/model"

type scanner interface {
	Scan(dest ...any) error
}

func scanPosition(row scanner) (model.Position, error) {
	var stored model.Position
	if err := row.Scan(&stored.ID, &stored.TripID, &stored.Latitude, &stored.Longitude, &stored.RecordedAt); err != nil {
		return model.Position{}, err
	}
	stored.RecordedAt = stored.RecordedAt.UTC()
	return stored, nil
}
