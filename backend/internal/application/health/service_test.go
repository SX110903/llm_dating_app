package health_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sx110903/llmatch-v2/backend/internal/application/health"
)

type stubChecker struct {
	name string
	err  error
}

func (s stubChecker) Name() string                { return s.name }
func (s stubChecker) Check(context.Context) error { return s.err }

func TestReadinessIsHealthyWhenEveryDependencyIsUp(t *testing.T) {
	service := health.NewService(time.Second,
		stubChecker{name: "postgres"},
		stubChecker{name: "redis"},
	)

	result := service.Readiness(context.Background())

	require.Equal(t, health.StatusHealthy, result.Status)
	assert.Equal(t, health.CheckUp, result.Checks["postgres"])
	assert.Equal(t, health.CheckUp, result.Checks["redis"])
}

func TestReadinessIsDegradedWithoutLeakingDependencyErrors(t *testing.T) {
	service := health.NewService(time.Second,
		stubChecker{name: "postgres"},
		stubChecker{name: "redis", err: errors.New("redis://user:secret@redis:6379")},
	)

	result := service.Readiness(context.Background())

	require.Equal(t, health.StatusDegraded, result.Status)
	assert.Equal(t, map[string]string{"postgres": "up", "redis": "down"}, result.Checks)
}

func TestReadinessHonorsItsTimeout(t *testing.T) {
	service := health.NewService(time.Millisecond, blockingChecker{name: "postgres"})

	result := service.Readiness(context.Background())

	require.Equal(t, health.StatusDegraded, result.Status)
}

type blockingChecker struct{ name string }

func (s blockingChecker) Name() string { return s.name }
func (s blockingChecker) Check(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
