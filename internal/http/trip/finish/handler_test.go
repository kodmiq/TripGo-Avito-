package finish_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/http/trip/finish"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func TestFinishTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMocktripFinisher(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc.EXPECT().Finish(gomock.Any(), id).Return(model.Trip{ID: id, Status: "completed"}, nil)

	rec := callFinish(t, finish.New(uc, slog.New(slog.DiscardHandler)), id, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"completed"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFinishTripCompleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMocktripFinisher(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uc.EXPECT().Finish(gomock.Any(), id).Return(model.Trip{}, domain.ErrTripCompleted)

	rec := callFinish(t, finish.New(uc, slog.New(slog.DiscardHandler)), id, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"trip_completed"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFinishTripUnexpectedBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	rec := callFinish(t, finish.New(NewMocktripFinisher(ctrl), slog.New(slog.DiscardHandler)), uuid.MustParse("11111111-1111-1111-1111-111111111111"), `{"price":1}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFinishTripRepositoryErrorHidesDetails(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMocktripFinisher(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	secret := "update failed: secret-token"
	uc.EXPECT().Finish(gomock.Any(), id).Return(model.Trip{}, errors.New(secret))

	rec := callFinish(t, finish.New(uc, slog.New(slog.DiscardHandler)), id, "")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"code":"internal_error"`) || strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func callFinish(t *testing.T, h *finish.Handler, id uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", bodyReader)
	rec := httptest.NewRecorder()
	h.FinishTrip(rec, req, id)
	return rec
}
