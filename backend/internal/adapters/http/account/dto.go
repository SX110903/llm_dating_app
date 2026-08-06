package account

type ConsentRequest struct {
	Purpose string `json:"purpose" validate:"required"`
}

type ConsentResponse struct {
	Purpose       string `json:"purpose"`
	PolicyVersion string `json:"policy_version"`
	GrantedAt     string `json:"granted_at"`
}

type ErrorResponse struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}
