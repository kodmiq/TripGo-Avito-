package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Tracing открывает корневой спан входящего HTTP-запроса через otelhttp.
// Имя спана — «{method} {route}», route берётся из шаблона chi, а не из URL.
func Tracing(tp trace.TracerProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			span := trace.SpanFromContext(r.Context())
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				span.SetStatus(codes.Error, "")
				span.SetAttributes(attribute.String("error.type", "500"))
				panic(recovered)
			}()
			next.ServeHTTP(rec, r)
			route := routePattern(r)
			span.SetName(r.Method + " " + route)
			span.SetAttributes(attribute.String("http.route", route))
			if rec.status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, "")
				span.SetAttributes(attribute.String("error.type", "500"))
			}
		})
		return otelhttp.NewMiddleware("http.request",
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + routePattern(r)
			}),
		)(inner)
	}
}

func routePattern(r *http.Request) string {
	route := chi.RouteContext(r.Context())
	if route == nil {
		return "unknown"
	}
	// RoutePattern заполняется по мере прохода по дереву chi и до вызова
	// next ещё пустой. Повторный поиск по тому же дереву даёт шаблон сразу,
	// чтобы +1 и -1 у active_requests шли с одним и тем же http.route.
	if pattern := route.RoutePattern(); pattern != "" {
		return pattern
	}
	if route.Routes == nil {
		return "unknown"
	}
	matched := chi.NewRouteContext()
	if !route.Routes.Match(matched, r.Method, r.URL.Path) {
		return "unknown"
	}
	if pattern := matched.RoutePattern(); pattern != "" {
		return pattern
	}
	return "unknown"
}
