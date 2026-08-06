package account

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	applicationaccount "github.com/sx110903/llmatch-v2/backend/internal/application/account"
	domainaccount "github.com/sx110903/llmatch-v2/backend/internal/domain/account"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

const (
	maxJSONBodyBytes = 16 * 1024
	consentSourceAPI = "api"
)

type Handler struct {
	service  *applicationaccount.PrivacyService
	validate *validator.Validate
}

func NewHandler(service *applicationaccount.PrivacyService) *Handler {
	return &Handler{service: service, validate: validator.New(validator.WithRequiredStructEnabled())}
}

func (h *Handler) GrantConsent(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	var req ConsentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}
	if req.Purpose != domainaccount.PurposeGenderPreferences {
		writeError(w, r, http.StatusUnprocessableEntity, "UNSUPPORTED_CONSENT_PURPOSE", "unknown consent purpose")
		return
	}

	consent, err := h.service.GrantGenderPreferenceConsent(r.Context(), identity.UserID, consentSourceAPI)
	if err != nil {
		if errors.Is(err, domainaccount.ErrConsentActive) {
			writeError(w, r, http.StatusConflict, "CONSENT_ALREADY_ACTIVE", "consent for this purpose is already active")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not grant consent")
		return
	}
	writeJSON(w, http.StatusCreated, consentResponse(*consent))
}

func (h *Handler) GetConsent(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	purpose := chi.URLParam(r, "purpose")
	if purpose != domainaccount.PurposeGenderPreferences {
		writeError(w, r, http.StatusUnprocessableEntity, "UNSUPPORTED_CONSENT_PURPOSE", "unknown consent purpose")
		return
	}

	consent, err := h.service.GetActiveGenderPreferenceConsent(r.Context(), identity.UserID)
	if err != nil {
		if errors.Is(err, domainaccount.ErrConsentNotFound) {
			writeError(w, r, http.StatusNotFound, "CONSENT_NOT_FOUND", "no active consent for this purpose")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load consent")
		return
	}
	writeJSON(w, http.StatusOK, consentResponse(*consent))
}

func (h *Handler) WithdrawConsent(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	if chi.URLParam(r, "purpose") != domainaccount.PurposeGenderPreferences {
		writeError(w, r, http.StatusUnprocessableEntity, "UNSUPPORTED_CONSENT_PURPOSE", "unknown consent purpose")
		return
	}
	if err := h.service.WithdrawGenderPreferenceConsent(r.Context(), identity.UserID); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not withdraw consent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func requireIdentity(w http.ResponseWriter, r *http.Request) (platformmiddleware.Identity, bool) {
	identity, ok := platformmiddleware.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing identity")
		return platformmiddleware.Identity{}, false
	}
	return identity, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func consentResponse(c domainaccount.Consent) ConsentResponse {
	return ConsentResponse{
		Purpose:       c.Purpose,
		PolicyVersion: c.PolicyVersion,
		GrantedAt:     c.GrantedAt.Format(time.RFC3339),
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: platformmiddleware.RequestIDFromContext(r.Context()),
	})
}
