package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder for magic-byte / dimension validation
	_ "image/png"  // register PNG decoder for magic-byte / dimension validation
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	_ "golang.org/x/image/webp" // register WebP decoder for magic-byte / dimension validation

	applicationprofile "github.com/sx110903/llmatch-v2/backend/internal/application/profile"
	domainprofile "github.com/sx110903/llmatch-v2/backend/internal/domain/profile"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

const (
	maxJSONBodyBytes = 256 * 1024
	maxPhotoBytes    = 10 * 1024 * 1024
	maxUploadBytes   = maxPhotoBytes + 64*1024 // multipart framing overhead
)

var allowedPhotoMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

type Handler struct {
	service   *applicationprofile.Service
	validate  *validator.Validate
	sanitizer *bluemonday.Policy
}

func NewHandler(service *applicationprofile.Service) *Handler {
	return &Handler{
		service:   service,
		validate:  validator.New(validator.WithRequiredStructEnabled()),
		sanitizer: bluemonday.StrictPolicy(),
	}
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	found, err := h.service.GetProfile(r.Context(), identity.UserID)
	if err != nil {
		if errors.Is(err, domainprofile.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "PROFILE_NOT_FOUND", "profile has not been created yet")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load profile")
		return
	}
	writeJSON(w, http.StatusOK, profileResponse(*found))
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	var req UpdateProfileRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}

	updated, err := h.service.UpdateProfile(r.Context(), identity.UserID, applicationprofile.UpdateProfileInput{
		Bio:                 h.sanitize(req.Bio),
		Interests:           h.sanitizeList(req.Interests),
		City:                h.sanitize(req.City),
		Latitude:            req.Latitude,
		Longitude:           req.Longitude,
		Questionnaire:       h.sanitizeQuestionnaire(req.Questionnaire),
		OnboardingCompleted: req.OnboardingCompleted,
	})
	if err != nil {
		if errors.Is(err, applicationprofile.ErrOnboardingIncomplete) {
			writeError(w, r, http.StatusUnprocessableEntity, "ONBOARDING_INCOMPLETE", "add a bio and at least one photo before completing onboarding")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update profile")
		return
	}
	writeJSON(w, http.StatusOK, profileResponse(*updated))
}

func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	found, err := h.service.GetPreferences(r.Context(), identity.UserID)
	if err != nil {
		if errors.Is(err, domainprofile.ErrPreferencesNotFound) {
			writeError(w, r, http.StatusNotFound, "PREFERENCES_NOT_FOUND", "preferences have not been created yet")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load preferences")
		return
	}
	writeJSON(w, http.StatusOK, preferencesResponse(*found))
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	var req UpdatePreferencesRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}

	var genders []string
	if req.Genders != nil {
		genders = *req.Genders
	}
	updated, err := h.service.UpdatePreferences(r.Context(), identity.UserID, applicationprofile.UpdatePreferencesInput{
		MinAge:        req.MinAge,
		MaxAge:        req.MaxAge,
		MaxDistanceKM: req.MaxDistanceKM,
		Genders:       genders,
	})
	if err != nil {
		switch {
		case errors.Is(err, domainprofile.ErrConsentRequired):
			writeError(w, r, http.StatusUnprocessableEntity, "CONSENT_REQUIRED", "grant consent for gender preferences before saving this field")
		case errors.Is(err, domainprofile.ErrInvalidAgeRange), errors.Is(err, domainprofile.ErrInvalidDistance):
			writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		default:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update preferences")
		}
		return
	}
	writeJSON(w, http.StatusOK, preferencesResponse(*updated))
}

func (h *Handler) ListPhotos(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	photos, err := h.service.ListPhotos(r.Context(), identity.UserID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not list photos")
		return
	}
	responses := make([]PhotoResponse, 0, len(photos))
	for _, p := range photos {
		responses = append(responses, photoResponse(p))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *Handler) CreatePhoto(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		writeError(w, r, http.StatusRequestEntityTooLarge, "PHOTO_TOO_LARGE", "photo exceeds the maximum allowed size")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, _, err := r.FormFile("photo")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "missing photo file field")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxPhotoBytes+1))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "could not read photo file")
		return
	}
	if len(data) > maxPhotoBytes {
		writeError(w, r, http.StatusRequestEntityTooLarge, "PHOTO_TOO_LARGE", "photo exceeds the maximum allowed size")
		return
	}

	mimeType, width, height, err := detectImage(data)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "UNSUPPORTED_MIME_TYPE", "photo must be a valid JPEG, PNG or WebP image")
		return
	}

	created, err := h.service.CreatePhoto(r.Context(), identity.UserID, applicationprofile.NewPhotoInput{
		MimeType: mimeType,
		Width:    width,
		Height:   height,
		ByteSize: int64(len(data)),
		Data:     data,
	})
	if err != nil {
		if errors.Is(err, domainprofile.ErrPhotoLimitReached) {
			writeError(w, r, http.StatusUnprocessableEntity, "PHOTO_LIMIT_REACHED", fmt.Sprintf("maximum of %d photos reached", domainprofile.MaxPhotos))
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not save photo")
		return
	}
	writeJSON(w, http.StatusCreated, photoResponse(*created))
}

func (h *Handler) GetPhotoContent(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	photoID, err := uuid.Parse(chi.URLParam(r, "photoID"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid photo id")
		return
	}

	reader, mimeType, err := h.service.GetPhotoContent(r.Context(), identity.UserID, photoID)
	if err != nil {
		if errors.Is(err, domainprofile.ErrPhotoNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "photo not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load photo")
		return
	}
	defer func() { _ = reader.Close() }()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func (h *Handler) ReorderPhotos(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	var req ReorderPhotosRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}

	ids := make([]uuid.UUID, 0, len(req.PhotoIDs))
	for _, raw := range req.PhotoIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid photo id")
			return
		}
		ids = append(ids, id)
	}

	if err := h.service.ReorderPhotos(r.Context(), identity.UserID, ids); err != nil {
		if errors.Is(err, applicationprofile.ErrInvalidPhotoOrder) {
			writeError(w, r, http.StatusUnprocessableEntity, "INVALID_PHOTO_ORDER", "photo order must match the user's current photos")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not reorder photos")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SetPrimaryPhoto(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	photoID, err := uuid.Parse(chi.URLParam(r, "photoID"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid photo id")
		return
	}
	if err := h.service.SetPrimaryPhoto(r.Context(), identity.UserID, photoID); err != nil {
		if errors.Is(err, domainprofile.ErrPhotoNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "photo not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update photo")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	photoID, err := uuid.Parse(chi.URLParam(r, "photoID"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid photo id")
		return
	}
	if err := h.service.DeletePhoto(r.Context(), identity.UserID, photoID); err != nil {
		if errors.Is(err, domainprofile.ErrPhotoNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "photo not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not delete photo")
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

func detectImage(data []byte) (mimeType string, width, height int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, fmt.Errorf("decode image: %w", err)
	}
	mimeType = formatToMimeType(format)
	if !allowedPhotoMimeTypes[mimeType] {
		return "", 0, 0, fmt.Errorf("unsupported image format: %s", format)
	}
	return mimeType, cfg.Width, cfg.Height, nil
}

func formatToMimeType(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func (h *Handler) sanitize(value string) string {
	return strings.TrimSpace(h.sanitizer.Sanitize(value))
}

func (h *Handler) sanitizeList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		if sanitized := h.sanitize(v); sanitized != "" {
			result = append(result, sanitized)
		}
	}
	return result
}

func (h *Handler) sanitizeQuestionnaire(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(value))
	for k, v := range value {
		result[k] = h.sanitizeJSONValue(v)
	}
	return result
}

func (h *Handler) sanitizeJSONValue(v any) any {
	switch val := v.(type) {
	case string:
		return h.sanitize(val)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = h.sanitizeJSONValue(item)
		}
		return result
	case map[string]any:
		return h.sanitizeQuestionnaire(val)
	default:
		return val
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func profileResponse(p domainprofile.Profile) ProfileResponse {
	return ProfileResponse{
		UserID:              p.UserID.String(),
		Bio:                 p.Bio,
		Interests:           p.Interests,
		City:                p.City,
		HasLocation:         p.HasLocation,
		Questionnaire:       p.Questionnaire,
		OnboardingCompleted: p.OnboardingCompleted,
		CreatedAt:           p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           p.UpdatedAt.Format(time.RFC3339),
	}
}

func preferencesResponse(p domainprofile.Preferences) PreferencesResponse {
	return PreferencesResponse{
		MinAge:        p.MinAge,
		MaxAge:        p.MaxAge,
		MaxDistanceKM: p.MaxDistanceKM,
		Genders:       p.Genders,
	}
}

func photoResponse(p domainprofile.Photo) PhotoResponse {
	return PhotoResponse{
		ID:        p.ID.String(),
		MimeType:  p.MimeType,
		ByteSize:  p.ByteSize,
		Width:     p.Width,
		Height:    p.Height,
		Position:  p.Position,
		IsPrimary: p.IsPrimary,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
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
