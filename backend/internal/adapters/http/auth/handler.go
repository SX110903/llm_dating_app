package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"

	applicationauth "github.com/sx110903/llmatch-v2/backend/internal/application/auth"
	domainuser "github.com/sx110903/llmatch-v2/backend/internal/domain/user"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

const refreshCookieName = "refresh_token"

type Handler struct {
	service        *applicationauth.Service
	validate       *validator.Validate
	sanitizer      *bluemonday.Policy
	cookiePath     string
	cookieSecure   bool
	allowedOrigins map[string]struct{}
}

func NewHandler(service *applicationauth.Service, production bool, cookiePath string, allowedOrigins []string) *Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return &Handler{
		service:        service,
		validate:       validator.New(validator.WithRequiredStructEnabled()),
		sanitizer:      bluemonday.StrictPolicy(),
		cookiePath:     cookiePath,
		cookieSecure:   production,
		allowedOrigins: allowed,
	}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}
	birthDate, err := time.Parse("2006-01-02", req.BirthDate)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "birth_date must be an ISO-8601 date")
		return
	}

	createdUser, err := h.service.Register(r.Context(), applicationauth.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: h.sanitize(req.DisplayName),
		BirthDate:   birthDate,
		Gender:      h.sanitize(req.Gender),
		IP:          clientIP(r),
	})
	if err != nil {
		h.handleRegisterError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, userResponse(*createdUser))
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}

	result, err := h.service.Login(r.Context(), applicationauth.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		h.handleAuthError(w, r, err)
		return
	}

	h.setRefreshCookie(w, result.RefreshToken, result.RefreshTokenExpiresAt)
	writeJSON(w, http.StatusOK, loginResponse(result))
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, r, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "origin not allowed")
		return
	}
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing refresh token")
		return
	}

	result, err := h.service.Refresh(r.Context(), applicationauth.RefreshInput{
		RefreshToken: cookie.Value,
		IP:           clientIP(r),
		UserAgent:    r.UserAgent(),
	})
	if err != nil {
		h.clearRefreshCookie(w)
		h.handleAuthError(w, r, err)
		return
	}

	h.setRefreshCookie(w, result.RefreshToken, result.RefreshTokenExpiresAt)
	writeJSON(w, http.StatusOK, loginResponse(result))
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, r, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "origin not allowed")
		return
	}
	identity, ok := platformmiddleware.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing identity")
		return
	}
	refreshValue := ""
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		refreshValue = cookie.Value
	}
	if err := h.service.Logout(r.Context(), identity.UserID, identity.JTI, identity.ExpiresAt, refreshValue); err != nil {
		h.handleAuthError(w, r, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, r, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "origin not allowed")
		return
	}
	identity, ok := platformmiddleware.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing identity")
		return
	}
	if err := h.service.LogoutAll(r.Context(), identity.UserID); err != nil {
		h.handleAuthError(w, r, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	identity, ok := platformmiddleware.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing identity")
		return
	}
	foundUser, err := h.service.Me(r.Context(), identity.UserID)
	if err != nil {
		if errors.Is(err, domainuser.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load user")
		return
	}
	writeJSON(w, http.StatusOK, userResponse(*foundUser))
}

func (h *Handler) handleRegisterError(w http.ResponseWriter, r *http.Request, err error) {
	var rateLimited *applicationauth.RateLimitedError
	switch {
	case errors.As(err, &rateLimited):
		w.Header().Set("Retry-After", strconv.Itoa(int(rateLimited.RetryAfter.Seconds())))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts")
	case errors.Is(err, domainuser.ErrEmailTaken):
		writeError(w, r, http.StatusConflict, "EMAIL_TAKEN", "email is already registered")
	case errors.Is(err, domainuser.ErrUnderage):
		writeError(w, r, http.StatusUnprocessableEntity, "UNDERAGE", "minimum age is 18")
	case errors.Is(err, applicationauth.ErrWeakPassword):
		writeError(w, r, http.StatusUnprocessableEntity, "WEAK_PASSWORD", "password does not meet the minimum policy")
	case errors.Is(err, applicationauth.ErrDependencyUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "AUTH_DEPENDENCY_UNAVAILABLE", "authentication dependency unavailable")
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not complete registration")
	}
}

func (h *Handler) handleAuthError(w http.ResponseWriter, r *http.Request, err error) {
	var rateLimited *applicationauth.RateLimitedError
	switch {
	case errors.As(err, &rateLimited):
		w.Header().Set("Retry-After", strconv.Itoa(int(rateLimited.RetryAfter.Seconds())))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts")
	case errors.Is(err, applicationauth.ErrInvalidCredentials):
		writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	case errors.Is(err, applicationauth.ErrInvalidRefreshToken):
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired refresh token")
	case errors.Is(err, applicationauth.ErrDependencyUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "AUTH_DEPENDENCY_UNAVAILABLE", "authentication dependency unavailable")
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected authentication error")
	}
}

func (h *Handler) sanitize(value string) string {
	return strings.TrimSpace(h.sanitizer.Sanitize(value))
}

// originAllowed enforces the plan's Origin allowlist check for refresh and
// logout. Requests without an Origin header (same-origin navigations, most
// non-browser clients) are allowed through; SameSite=Strict already blocks
// the cross-site case those requests could represent.
func (h *Handler) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	_, ok := h.allowedOrigins[origin]
	return ok
}

// #nosec G124 -- Secure is intentionally driven by h.cookieSecure (true in
// production, false only for local HTTP development); HttpOnly and
// SameSite=Strict are always set.
func (h *Handler) setRefreshCookie(w http.ResponseWriter, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     h.cookiePath,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
	})
}

// #nosec G124 -- see setRefreshCookie.
func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     h.cookiePath,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func decodeJSON(r *http.Request, dest any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func loginResponse(result *applicationauth.AuthResult) LoginResponse {
	return LoginResponse{
		AccessToken:          result.AccessToken,
		AccessTokenExpiresAt: result.AccessTokenExpiresAt.Format(time.RFC3339),
		User:                 userResponse(result.User),
	}
}

func userResponse(u domainuser.User) UserResponse {
	var emailVerifiedAt *string
	if u.EmailVerifiedAt != nil {
		formatted := u.EmailVerifiedAt.Format(time.RFC3339)
		emailVerifiedAt = &formatted
	}
	return UserResponse{
		ID:              u.ID.String(),
		Email:           u.Email,
		DisplayName:     u.DisplayName,
		BirthDate:       u.BirthDate.Format("2006-01-02"),
		Gender:          u.Gender,
		Status:          string(u.Status),
		EmailVerifiedAt: emailVerifiedAt,
		CreatedAt:       u.CreatedAt.Format(time.RFC3339),
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
