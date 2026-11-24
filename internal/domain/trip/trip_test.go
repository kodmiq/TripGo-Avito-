package trip

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFinish(t *testing.T) {
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	earlier := at.Add(-time.Hour)

	tests := []struct {
		name     string
		status   Status
		finished *time.Time
		wantErr  error
	}{
		{name: "active becomes completed", status: StatusActive},
		{name: "completed stays completed", status: StatusCompleted, finished: &earlier, wantErr: ErrTripCompleted},
		{name: "unknown status is rejected", status: "paused", wantErr: ErrTripCompleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := Trip{Status: tt.status, FinishedAt: tt.finished}
			err := entity.Finish(at)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if entity.Status != tt.status {
					t.Fatalf("status changed to %q", entity.Status)
				}
				if tt.finished != nil && (entity.FinishedAt == nil || !entity.FinishedAt.Equal(*tt.finished)) {
					t.Fatalf("finished_at changed to %v", entity.FinishedAt)
				}
				return
			}
			if entity.Status != StatusCompleted {
				t.Fatalf("status = %q", entity.Status)
			}
			if entity.FinishedAt == nil || !entity.FinishedAt.Equal(at) {
				t.Fatalf("finished_at = %v, want %v", entity.FinishedAt, at)
			}
		})
	}
}

func TestValidateCreate(t *testing.T) {
	userID := uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c")
	driverID := uuid.MustParse("6cb72c04-7650-45c9-a79b-bcdba0631e0c")

	tests := []struct {
		name               string
		userID, driverID   uuid.UUID
		startLat, startLon float64
		endLat, endLon     float64
		price              int64
		wantDetail         string
	}{
		{name: "ok", userID: userID, driverID: driverID, startLat: 59.9, startLon: 30.3, endLat: 59.8, endLon: 30.4, price: 0},
		{name: "edges", userID: userID, driverID: driverID, startLat: -90, startLon: -180, endLat: 90, endLon: 180, price: 10},
		{name: "empty user", driverID: driverID, startLat: 1, startLon: 2, endLat: 3, endLon: 4, wantDetail: "user_id must be a non-empty UUID"},
		{name: "empty driver", userID: userID, startLat: 1, startLon: 2, endLat: 3, endLon: 4, wantDetail: "driver_id must be a non-empty UUID"},
		{name: "start latitude", userID: userID, driverID: driverID, startLat: 90.1, startLon: 0, endLat: 1, endLon: 2, wantDetail: "start_point.latitude must be between -90 and 90"},
		{name: "start longitude", userID: userID, driverID: driverID, startLat: 0, startLon: -180.1, endLat: 1, endLon: 2, wantDetail: "start_point.longitude must be between -180 and 180"},
		{name: "end latitude", userID: userID, driverID: driverID, startLat: 1, startLon: 2, endLat: -90.1, endLon: 0, wantDetail: "end_point.latitude must be between -90 and 90"},
		{name: "end longitude", userID: userID, driverID: driverID, startLat: 1, startLon: 2, endLat: 0, endLon: 180.1, wantDetail: "end_point.longitude must be between -180 and 180"},
		{name: "negative price", userID: userID, driverID: driverID, startLat: 1, startLon: 2, endLat: 3, endLon: 4, price: -1, wantDetail: "price must be greater than or equal to 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreate(tt.userID, tt.driverID, tt.startLat, tt.startLon, tt.endLat, tt.endLon, tt.price)
			if tt.wantDetail == "" {
				if err != nil {
					t.Fatalf("ValidateCreate() = %v", err)
				}
				return
			}
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Detail != tt.wantDetail {
				t.Fatalf("ValidateCreate() = %v, want detail %q", err, tt.wantDetail)
			}
		})
	}
}

func TestNewActive(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userID := uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c")
	driverID := uuid.MustParse("6cb72c04-7650-45c9-a79b-bcdba0631e0c")
	started := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	got, err := NewActive(id, userID, driverID, 59.9, 30.3, 59.8, 30.4, 1450, started)
	if err != nil {
		t.Fatalf("NewActive() = %v", err)
	}
	if got.Status != StatusActive || got.FinishedAt != nil || !got.StartedAt.Equal(started) || got.ID != id || got.Price != 1450 {
		t.Fatalf("NewActive() = %+v", got)
	}

	if _, err := NewActive(id, uuid.Nil, driverID, 1, 2, 3, 4, 1, started); !errors.As(err, new(*ValidationError)) {
		t.Fatalf("NewActive() error = %v, want validation", err)
	}
}
