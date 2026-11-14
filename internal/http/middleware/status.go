package middleware

import "net/http"

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(body []byte) (int, error) {
	if !s.wrote {
		s.status = http.StatusOK
		s.wrote = true
	}
	return s.ResponseWriter.Write(body)
}

// finishStatus — код, который записывают лог, метрика и спан.
// Паника до WriteHeader означает ответ 500: заголовок отправит Recover.
func finishStatus(rec *statusRecorder, panicked bool) int {
	if panicked && !rec.wrote {
		rec.status = http.StatusInternalServerError
	}
	return rec.status
}
