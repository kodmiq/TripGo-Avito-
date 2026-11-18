package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const pushPath = "/api/v1/push"

// StatusError — Push Service ответил кодом 4xx или 5xx. Это не ошибка транспорта.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	if e.Code >= http.StatusInternalServerError {
		return fmt.Sprintf("push service unavailable: status %d", e.Code)
	}
	return fmt.Sprintf("push service rejected request: status %d", e.Code)
}

// Client ходит в Push Service своим http.Client. Таймаут задаётся и на клиенте, и контекстом задачи.
type Client struct {
	url    string
	http   *http.Client
	tracer trace.Tracer
}

func New(baseURL string, timeout time.Duration, tracer trace.Tracer) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("push base url is empty")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("push timeout must be greater than 0")
	}
	if tracer == nil {
		tracer = noop.NewTracerProvider().Tracer("trip-service")
	}
	return &Client{
		url:    baseURL + pushPath,
		tracer: tracer,
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   timeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
				ExpectContinueTimeout: time.Second,
			},
		},
	}, nil
}

// Close закрывает простаивающие соединения. Вызывается после остановки воркеров.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// RequestPosition просит устройство водителя прислать координату поездки.
func (c *Client) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	ctx, span := c.tracer.Start(ctx, "push.request", trace.WithAttributes(
		attribute.String("trip_id", tripID.String()),
		attribute.String("http.request.method", http.MethodPost),
	))
	defer span.End()

	body, err := json.Marshal(pushBody{
		RequestID:   uuid.NewString(),
		RecipientID: driverID.String(),
		Kind:        "REQUEST_POSITION",
		Data:        map[string]string{"trip_id": tripID.String()},
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "encode")
		return fmt.Errorf("encode push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "request")
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.http.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "transport")
		return fmt.Errorf("push transport: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusAccepted {
		return nil
	}
	statusErr := &StatusError{Code: resp.StatusCode}
	span.RecordError(statusErr)
	span.SetStatus(codes.Error, statusErr.Error())
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	return statusErr
}

type pushBody struct {
	RequestID   string            `json:"request_id"`
	RecipientID string            `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}
