package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httphealth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/health"
	applicationhealth "github.com/sx110903/llmatch-v2/backend/internal/application/health"
)

type checker struct{ err error }

func (checker) Name() string                  { return "postgres" }
func (c checker) Check(context.Context) error { return c.err }

func TestLivenessReturnsOK(t *testing.T) {
	handler := httphealth.NewHandler(applicationhealth.NewService(time.Second))
	response := httptest.NewRecorder()

	handler.Live(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil))

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"healthy","checks":{"process":"up"}}`, response.Body.String())
}

func TestReadinessReturnsServiceUnavailable(t *testing.T) {
	service := applicationhealth.NewService(time.Second, checker{err: errors.New("unavailable")})
	handler := httphealth.NewHandler(service)
	response := httptest.NewRecorder()

	handler.Ready(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/ready", nil))

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"status":"degraded","checks":{"postgres":"down"}}`, response.Body.String())
}
