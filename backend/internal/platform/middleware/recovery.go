package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"
)

func Recover(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error().
						Interface("panic", recovered).
						Str("request_id", RequestIDFromContext(r.Context())).
						Msg("request panic recovered")
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"code":       "INTERNAL_ERROR",
						"message":    "internal server error",
						"request_id": RequestIDFromContext(r.Context()),
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
