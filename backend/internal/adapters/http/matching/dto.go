package matching

type SwipeRequest struct {
	TargetID string `json:"target_id" validate:"required,uuid"`
	Action   string `json:"action" validate:"required,oneof=like dislike superlike"`
}

type BlockRequest struct {
	UserID string `json:"user_id" validate:"required,uuid"`
}

type ReportRequest struct {
	UserID      string `json:"user_id" validate:"required,uuid"`
	Reason      string `json:"reason" validate:"required,oneof=harassment spam inappropriate_content impersonation underage other"`
	Description string `json:"description" validate:"max=1000"`
}

type ScoreResponse struct {
	Interests     float64 `json:"interests"`
	Questionnaire float64 `json:"questionnaire"`
	Distance      float64 `json:"distance"`
	Activity      float64 `json:"activity"`
	Total         float64 `json:"total"`
}

type CandidateResponse struct {
	UserID       string        `json:"user_id"`
	DisplayName  string        `json:"display_name"`
	Age          int           `json:"age"`
	Gender       string        `json:"gender"`
	Bio          string        `json:"bio"`
	Interests    []string      `json:"interests"`
	City         string        `json:"city"`
	DistanceKM   float64       `json:"distance_km"`
	LastActiveAt string        `json:"last_active_at"`
	PhotoURL     string        `json:"photo_url"`
	Score        ScoreResponse `json:"score"`
}

type DiscoveryResponse struct {
	Candidates []CandidateResponse `json:"candidates"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type CreatedMatchResponse struct {
	ID        string `json:"id"`
	MatchedAt string `json:"matched_at"`
}

type SwipeResponse struct {
	ID        string                `json:"id"`
	TargetID  string                `json:"target_id"`
	Action    string                `json:"action"`
	CreatedAt string                `json:"created_at"`
	Match     *CreatedMatchResponse `json:"match,omitempty"`
}

type MatchResponse struct {
	ID           string `json:"id"`
	OtherUserID  string `json:"other_user_id"`
	DisplayName  string `json:"display_name"`
	Bio          string `json:"bio"`
	City         string `json:"city"`
	PhotoURL     string `json:"photo_url"`
	MatchedAt    string `json:"matched_at"`
	LastActiveAt string `json:"last_active_at"`
}

type MatchListResponse struct {
	Matches    []MatchResponse `json:"matches"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type ReportResponse struct {
	ID          string `json:"id"`
	ReportedID  string `json:"reported_id"`
	Reason      string `json:"reason"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type ErrorResponse struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}
