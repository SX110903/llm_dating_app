package httpadapter

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	httpauth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/auth"
	httphealth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/health"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

type RouterConfig struct {
	Logger         zerolog.Logger
	AllowedOrigins []string
	Production     bool
	Health         *httphealth.Handler
	Auth           *httpauth.Handler
	AuthMiddleware func(http.Handler) http.Handler
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
	router.Route("/api/v1", func(r chi.Router) {
		registerHealth(r)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", config.Auth.Register)
			r.Post("/login", config.Auth.Login)
			r.Post("/refresh", config.Auth.Refresh)
			r.Group(func(r chi.Router) {
				r.Use(config.AuthMiddleware)
				r.Post("/logout", config.Auth.Logout)
				r.Post("/logout-all", config.Auth.LogoutAll)
				r.Get("/me", config.Auth.Me)
			})
		})
	})

	return router
}
