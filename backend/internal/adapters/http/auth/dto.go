package auth

type RegisterRequest struct {
	Email       string `json:"email" validate:"required,email,max=255"`
	Password    string `json:"password" validate:"required,min=12,max=128"`
	DisplayName string `json:"display_name" validate:"required,min=1,max=100"`
	BirthDate   string `json:"birth_date" validate:"required,datetime=2006-01-02"`
	Gender      string `json:"gender" validate:"required,min=1,max=50"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,max=128"`
}

type UserResponse struct {
	ID              string  `json:"id"`
	Email           string  `json:"email"`
	DisplayName     string  `json:"display_name"`
	BirthDate       string  `json:"birth_date"`
	Gender          string  `json:"gender"`
	Status          string  `json:"status"`
	EmailVerifiedAt *string `json:"email_verified_at"`
	CreatedAt       string  `json:"created_at"`
}

type LoginResponse struct {
	AccessToken          string       `json:"access_token"`
	AccessTokenExpiresAt string       `json:"access_token_expires_at"`
	User                 UserResponse `json:"user"`
}

type ErrorResponse struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}
