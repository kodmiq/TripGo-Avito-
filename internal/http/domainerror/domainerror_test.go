package domainerror

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
)

func TestProblem(t *testing.T) {
	secret := "pq: password=secret-db-password"
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
		hidden     string
	}{
		{
			name:       "validation",
			err:        &domain.ValidationError{Detail: "price is required"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantDetail: "price is required",
		},
		{
			name:       "wrapped validation",
			err:        fmt.Errorf("decode: %w", &domain.ValidationError{Detail: "user_id is required"}),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantDetail: "user_id is required",
		},
		{
			name:       "driver busy",
			err:        fmt.Errorf("insert trip: %w", domain.ErrDriverBusy),
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
			wantDetail: "Driver already has an active trip",
		},
		{
			name:       "trip completed",
			err:        domain.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
			wantDetail: "Operation is not allowed for a completed trip",
		},
		{
			name:       "idempotency conflict",
			err:        domain.ErrIdempotencyConflict,
			wantStatus: http.StatusConflict,
			wantCode:   "idempotency_conflict",
			wantDetail: "Idempotency-Key was already used with a different request body",
		},
		{
			name:       "not found",
			err:        domain.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
			wantDetail: "Trip was not found",
		},
		{
			name:       "repository failure hides internals",
			err:        errors.New(secret),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantDetail: "Internal server error",
			hidden:     secret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := Problem(tt.err)
			if status != tt.wantStatus || body.Status != int32(tt.wantStatus) { //nolint:gosec // G115: код HTTP-ответа помещается в int32
				t.Fatalf("status = %d, body status = %d, want %d", status, body.Status, tt.wantStatus)
			}
			if body.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if body.Detail == nil || *body.Detail != tt.wantDetail {
				t.Fatalf("detail = %v, want %q", body.Detail, tt.wantDetail)
			}
			if tt.hidden != "" && (*body.Detail == tt.hidden || body.Title == tt.hidden) {
				t.Fatalf("response leaked %q", tt.hidden)
			}
		})
	}
}

func TestInvalidRequest(t *testing.T) {
	status, body := InvalidRequest("Request validation failed")
	if status != http.StatusBadRequest || body.Code != "invalid_request" || body.Detail == nil || *body.Detail != "Request validation failed" {
		t.Fatalf("InvalidRequest() = %d %+v", status, body)
	}
}
