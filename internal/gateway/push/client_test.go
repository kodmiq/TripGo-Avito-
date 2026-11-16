package push

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRequestPositionAccepted(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/push" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	client, err := New(server.URL, time.Second, nil)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	t.Cleanup(client.Close)

	driverID := uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5")
	tripID := uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")
	if err := client.RequestPosition(context.Background(), driverID, tripID); err != nil {
		t.Fatalf("RequestPosition() = %v", err)
	}
	if !strings.Contains(gotBody, `"kind":"REQUEST_POSITION"`) || !strings.Contains(gotBody, tripID.String()) || !strings.Contains(gotBody, driverID.String()) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestRequestPositionDistinguishesStatusAndTransport(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "bad request", status: http.StatusBadRequest},
		{name: "unavailable", status: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(server.Close)
			client, err := New(server.URL, time.Second, nil)
			if err != nil {
				t.Fatalf("New() = %v", err)
			}
			err = client.RequestPosition(context.Background(), uuid.New(), uuid.New())
			var status *StatusError
			if !errors.As(err, &status) || status.Code != tt.status {
				t.Fatalf("RequestPosition() = %v", err)
			}
			if strings.Contains(err.Error(), "transport") {
				t.Fatalf("status error looks like transport: %v", err)
			}
		})
	}
}

func TestRequestPositionTransportError(t *testing.T) {
	client, err := New("http://127.0.0.1:1", 50*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	err = client.RequestPosition(context.Background(), uuid.New(), uuid.New())
	var status *StatusError
	if err == nil || errors.As(err, &status) || !strings.Contains(err.Error(), "push transport") {
		t.Fatalf("RequestPosition() = %v", err)
	}
}

func TestRequestPositionClosesBody(t *testing.T) {
	client, err := New("http://push.example", time.Second, nil)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	body := &trackingBody{Reader: strings.NewReader(`{"message_id":"1"}`)}
	client.http.Transport = roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       body,
			Header:     make(http.Header),
		}, nil
	})
	if err := client.RequestPosition(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("RequestPosition() = %v", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func TestNewRejectsEmptyURL(t *testing.T) {
	if _, err := New("  ", time.Second, nil); err == nil {
		t.Fatal("New() accepted an empty url")
	}
}

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
