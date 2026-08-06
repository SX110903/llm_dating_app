package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Hijack forwards to the underlying writer. Without it, wrapping the writer
// here would silently strip the capability a WebSocket upgrade depends on.
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

// Flush keeps streaming responses working through the wrapper.
func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func AccessLog(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			logger.Info().
				Str("request_id", RequestIDFromContext(r.Context())).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", recorder.status).
				Dur("duration", time.Since(started)).
				Msg("request completed")
		})
	}
}
