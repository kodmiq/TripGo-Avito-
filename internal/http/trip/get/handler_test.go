package get_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/http/trip/get"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func TestGetTripNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMocktripReader(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc.EXPECT().Get(gomock.Any(), id).Return(model.Trip{}, domain.ErrTripNotFound)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)
	rec := httptest.NewRecorder()
	get.New(uc, slog.New(slog.DiscardHandler)).GetTrip(rec, req, id)

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"trip_not_found"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGetTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMocktripReader(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc.EXPECT().Get(gomock.Any(), id).Return(model.Trip{ID: id, Status: "active"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)
	rec := httptest.NewRecorder()
	get.New(uc, slog.New(slog.DiscardHandler)).GetTrip(rec, req, id)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), id.String()) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
