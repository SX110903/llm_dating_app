package httpadapter

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	httphealth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/health"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

type RouterConfig struct {
	Logger         zerolog.Logger
	AllowedOrigins []string
	Production     bool
	Health         *httphealth.Handler
}

func NewRouter(config RouterConfig) http.Handler {
	router := chi.NewRouter()
	router.Use(platformmiddleware.RequestID)
	router.Use(platformmiddleware.Recover(config.Logger))
	router.Use(platformmiddleware.AccessLog(config.Logger))
	router.Use(platformmiddleware.SecurityHeaders(config.Production))
	router.Use(platformmiddleware.CORS(config.AllowedOrigins))

	registerHealth := func(r chi.Router) {
		r.Get("/health/live", config.Health.Live)
		r.Get("/health/ready", config.Health.Ready)
		r.Get("/health", config.Health.Ready)
	}
	registerHealth(router)
	router.Route("/api/v1", registerHealth)

	return router
}
