package matching

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"

	applicationmatching "github.com/sx110903/llmatch-v2/backend/internal/application/matching"
	domainmatching "github.com/sx110903/llmatch-v2/backend/internal/domain/matching"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

const maxJSONBodyBytes = 64 * 1024

type Handler struct {
	service   *applicationmatching.Service
	validate  *validator.Validate
	sanitizer *bluemonday.Policy
}

func NewHandler(service *applicationmatching.Service) *Handler {
	return &Handler{
		service:   service,
		validate:  validator.New(validator.WithRequiredStructEnabled()),
		sanitizer: bluemonday.StrictPolicy(),
	}
}

func (h *Handler) Discover(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be a positive integer")
		return
	}
	page, err := h.service.Discover(r.Context(), identity.UserID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response := DiscoveryResponse{
		Candidates: make([]CandidateResponse, 0, len(page.Candidates)),
		NextCursor: page.NextCursor,
	}
	for _, candidate := range page.Candidates {
		response.Candidates = append(response.Candidates, candidateResponse(candidate))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) Swipe(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	var req SwipeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}
	targetID, err := uuid.Parse(req.TargetID)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid target id")
		return
	}

	outcome, err := h.service.Swipe(r.Context(), identity.UserID, applicationmatching.SwipeInput{
		TargetID: targetID,
		Action:   domainmatching.SwipeAction(req.Action),
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	status := http.StatusOK
	if outcome.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, swipeResponse(*outcome))
}

func (h *Handler) ListMatches(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be a positive integer")
		return
	}
	page, err := h.service.ListMatches(r.Context(), identity.UserID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response := MatchListResponse{Matches: make([]MatchResponse, 0, len(page.Matches)), NextCursor: page.NextCursor}
	for _, match := range page.Matches {
		response.Matches = append(response.Matches, matchResponse(match))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) Unmatch(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	matchID, err := uuid.Parse(chi.URLParam(r, "matchID"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid match id")
		return
	}
	if err := h.service.Unmatch(r.Context(), identity.UserID, matchID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Block(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	var req BlockRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}
	blockedID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
		return
	}
	if err := h.service.Block(r.Context(), identity.UserID, blockedID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	var req ReportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_BODY", "request body could not be parsed")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request failed validation")
		return
	}
	reportedID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
		return
	}
	report, err := h.service.Report(r.Context(), identity.UserID, applicationmatching.ReportInput{
		ReportedID:  reportedID,
		Reason:      domainmatching.ReportReason(req.Reason),
		Description: strings.TrimSpace(h.sanitizer.Sanitize(req.Description)),
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, reportResponse(*report))
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
		writeServiceError(w, r, err)
		return
	}
	defer func() { _ = reader.Close() }()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func parseLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, applicationmatching.ErrInvalidPageSize
	}
	return limit, nil
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
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var dailyLimit *applicationmatching.DailyLimitError
	switch {
	case errors.As(err, &dailyLimit):
		retryAfter := max(1, int(math.Ceil(dailyLimit.RetryAfter.Seconds())))
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeError(w, r, http.StatusTooManyRequests, "DAILY_SWIPE_LIMIT", "daily swipe limit reached")
	case errors.Is(err, applicationmatching.ErrInvalidCursor), errors.Is(err, applicationmatching.ErrInvalidPageSize):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid pagination parameters")
	case errors.Is(err, domainmatching.ErrDiscoveryNotReady):
		writeError(w, r, http.StatusUnprocessableEntity, "DISCOVERY_NOT_READY", "complete profile, preferences, consent, location and primary photo first")
	case errors.Is(err, domainmatching.ErrInvalidSwipeAction), errors.Is(err, domainmatching.ErrInvalidReportReason),
		errors.Is(err, domainmatching.ErrReportTooLong), errors.Is(err, domainmatching.ErrSelfInteraction):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, domainmatching.ErrNotFound), errors.Is(err, domainmatching.ErrSwipeNotFound),
		errors.Is(err, domainmatching.ErrMatchNotFound), errors.Is(err, domainmatching.ErrPhotoNotFound),
		errors.Is(err, domainmatching.ErrInteractionBlocked):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "matching resource not found")
	case errors.Is(err, domainmatching.ErrDependencyUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "matching is temporarily unavailable")
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "matching request could not be completed")
	}
}

func candidateResponse(candidate domainmatching.Candidate) CandidateResponse {
	return CandidateResponse{
		UserID:       candidate.UserID.String(),
		DisplayName:  candidate.DisplayName,
		Age:          candidate.Age,
		Gender:       candidate.Gender,
		Bio:          candidate.Bio,
		Interests:    candidate.Interests,
		City:         candidate.City,
		DistanceKM:   candidate.DistanceKM,
		LastActiveAt: candidate.LastActiveAt.Format(time.RFC3339),
		PhotoURL:     matchingPhotoURL(candidate.PrimaryPhotoID),
		Score: ScoreResponse{
			Interests:     candidate.Score.Interests,
			Questionnaire: candidate.Score.Questionnaire,
			Distance:      candidate.Score.Distance,
			Activity:      candidate.Score.Activity,
			Total:         candidate.Score.Total,
		},
	}
}

func swipeResponse(outcome domainmatching.SwipeOutcome) SwipeResponse {
	response := SwipeResponse{
		ID:        outcome.Swipe.ID.String(),
		TargetID:  outcome.Swipe.TargetID.String(),
		Action:    string(outcome.Swipe.Action),
		CreatedAt: outcome.Swipe.CreatedAt.Format(time.RFC3339),
	}
	if outcome.Match != nil {
		response.Match = &CreatedMatchResponse{
			ID: outcome.Match.ID.String(), MatchedAt: outcome.Match.MatchedAt.Format(time.RFC3339),
		}
	}
	return response
}

func matchResponse(match domainmatching.MatchSummary) MatchResponse {
	return MatchResponse{
		ID:           match.Match.ID.String(),
		OtherUserID:  match.OtherUserID.String(),
		DisplayName:  match.DisplayName,
		Bio:          match.Bio,
		City:         match.City,
		PhotoURL:     matchingPhotoURL(match.PrimaryPhotoID),
		MatchedAt:    match.Match.MatchedAt.Format(time.RFC3339),
		LastActiveAt: match.OtherLastActive.Format(time.RFC3339),
	}
}

func reportResponse(report domainmatching.Report) ReportResponse {
	return ReportResponse{
		ID:          report.ID.String(),
		ReportedID:  report.ReportedID.String(),
		Reason:      string(report.Reason),
		Description: report.Description,
		Status:      string(report.Status),
		CreatedAt:   report.CreatedAt.Format(time.RFC3339),
	}
}

func matchingPhotoURL(photoID uuid.UUID) string {
	return "/api/v1/matching/photos/" + photoID.String() + "/content"
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
