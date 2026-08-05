package health

import (
	"encoding/json"
	"net/http"

	applicationhealth "github.com/sx110903/llmatch-v2/backend/internal/application/health"
)

type Handler struct {
	service *applicationhealth.Service
}

func NewHandler(service *applicationhealth.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.service.Liveness())
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	result := h.service.Readiness(r.Context())
	status := http.StatusOK
	if result.Status != applicationhealth.StatusHealthy {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
