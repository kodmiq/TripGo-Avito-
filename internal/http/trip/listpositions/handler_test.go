package listpositions_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/http/trip/listpositions"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func TestListPositions(t *testing.T) {
	ctrl := gomock.NewController(t)
	list := NewMocklister(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	recorded := time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)
	list.EXPECT().List(gomock.Any(), id).Return([]model.Position{{
		ID: 42, TripID: id, Latitude: 59.935, Longitude: 30.335, RecordedAt: recorded,
	}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String()+"/positions", nil)
	rec := httptest.NewRecorder()
	listpositions.New(list, slog.New(slog.DiscardHandler)).ListTripPositions(rec, req, id)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"id":42`) || !strings.Contains(body, `"positions"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, body)
	}
}

func TestListPositionsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	list := NewMocklister(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	list.EXPECT().List(gomock.Any(), id).Return(nil, domain.ErrTripNotFound)

	req := httptest.NewRequest(http.MethodGet, "/positions", nil)
	rec := httptest.NewRecorder()
	listpositions.New(list, slog.New(slog.DiscardHandler)).ListTripPositions(rec, req, id)

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"trip_not_found"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestListPositionsEmptyArray(t *testing.T) {
	ctrl := gomock.NewController(t)
	list := NewMocklister(ctrl)
	id := uuid.New()
	list.EXPECT().List(gomock.Any(), id).Return(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/positions", nil)
	rec := httptest.NewRecorder()
	listpositions.New(list, slog.New(slog.DiscardHandler)).ListTripPositions(rec, req, id)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"positions":[]`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
