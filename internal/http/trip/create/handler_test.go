package create_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/trip/create"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
	"go.uber.org/mock/gomock"
)

const validBody = `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"6cb72c04-7650-45c9-a79b-bcdba0631e0c","start_point":{"latitude":59.9398,"longitude":30.3146},"end_point":{"latitude":59.9290,"longitude":30.3626},"price":1450}`

func TestCreateTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMockcreator(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc.EXPECT().Create(gomock.Any(), gomock.Any(), (*uuid.UUID)(nil)).Return(model.Trip{
		ID: id, Status: "active", Price: 1450,
		UserID:   uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"),
		DriverID: uuid.MustParse("6cb72c04-7650-45c9-a79b-bcdba0631e0c"),
	}, true, nil)

	rec := callCreate(t, create.New(uc, slog.New(slog.DiscardHandler)), validBody, "application/json", api.CreateTripParams{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/v1/trips/"+id.String() {
		t.Fatalf("location = %q", rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), `"finished_at":null`) || !strings.Contains(rec.Body.String(), `"last_position_at":null`) {
		t.Fatalf("null fields missing: %s", rec.Body.String())
	}
}

func TestCreateTripReplay(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMockcreator(ctrl)
	key := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	apiKey := api.IdempotencyKey(key)
	uc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ model.CreateTrip, got *uuid.UUID) (model.Trip, bool, error) {
			if got == nil || *got != key {
				t.Fatalf("idempotency key = %v", got)
			}
			return model.Trip{ID: key, Status: "active"}, false, nil
		},
	)

	rec := callCreate(t, create.New(uc, slog.New(slog.DiscardHandler)), validBody, "application/json", api.CreateTripParams{IdempotencyKey: &apiKey})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "" {
		t.Fatalf("replay set Location %q", rec.Header().Get("Location"))
	}
}

func TestCreateTripDriverBusy(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMockcreator(ctrl)
	uc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(model.Trip{}, false, domain.ErrDriverBusy)

	rec := callCreate(t, create.New(uc, slog.New(slog.DiscardHandler)), validBody, "application/json", api.CreateTripParams{})
	assertProblem(t, rec, http.StatusConflict, "driver_busy", "")
}

func TestCreateTripRepositoryErrorHidesDetails(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMockcreator(ctrl)
	secret := "pq: password=secret-db-password"
	uc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(model.Trip{}, false, errors.New(secret))

	rec := callCreate(t, create.New(uc, slog.New(slog.DiscardHandler)), validBody, "application/json", api.CreateTripParams{})
	assertProblem(t, rec, http.StatusInternalServerError, "internal_error", secret)
}

func TestCreateTripInvalidBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		wantDetail  string
	}{
		{name: "content type", body: validBody, contentType: "text/plain", wantDetail: "Content-Type must be application/json"},
		{name: "empty", body: "", contentType: "application/json", wantDetail: "request body is required"},
		{name: "missing price", body: `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"6cb72c04-7650-45c9-a79b-bcdba0631e0c","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4}}`, contentType: "application/json", wantDetail: "price is required"},
		{name: "unknown field", body: `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"6cb72c04-7650-45c9-a79b-bcdba0631e0c","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1,"extra":true}`, contentType: "application/json", wantDetail: "unknown field in request body"},
		{name: "latitude", body: `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"6cb72c04-7650-45c9-a79b-bcdba0631e0c","start_point":{"latitude":100,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`, contentType: "application/json", wantDetail: "start_point.latitude must be between -90 and 90"},
		{name: "negative price", body: `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"6cb72c04-7650-45c9-a79b-bcdba0631e0c","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":-1}`, contentType: "application/json", wantDetail: "price must be greater than or equal to 0"},
		{name: "empty user", body: `{"user_id":"00000000-0000-0000-0000-000000000000","driver_id":"6cb72c04-7650-45c9-a79b-bcdba0631e0c","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`, contentType: "application/json", wantDetail: "user_id must be a non-empty UUID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			rec := callCreate(t, create.New(NewMockcreator(ctrl), slog.New(slog.DiscardHandler)), tt.body, tt.contentType, api.CreateTripParams{})
			assertProblem(t, rec, http.StatusBadRequest, "invalid_request", "")
			if !strings.Contains(rec.Body.String(), tt.wantDetail) {
				t.Fatalf("body = %s, want detail %q", rec.Body.String(), tt.wantDetail)
			}
		})
	}
}

func callCreate(t *testing.T, h *create.Handler, body, contentType string, params api.CreateTripParams) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.CreateTrip(rec, req, params)
	return rec
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, code, hidden string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("body = %s, want code %s", rec.Body.String(), code)
	}
	if hidden != "" && strings.Contains(rec.Body.String(), hidden) {
		t.Fatalf("body leaked %q: %s", hidden, rec.Body.String())
	}
}
