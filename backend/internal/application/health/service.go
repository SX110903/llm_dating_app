package health

import (
	"context"
	"sync"
	"time"
)

const (
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"
	CheckUp        = "up"
	CheckDown      = "down"
)

// Checker is an outbound application port implemented by infrastructure adapters.
type Checker interface {
	Name() string
	Check(context.Context) error
}

type Result struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

type Service struct {
	checkers []Checker
	timeout  time.Duration
}

func NewService(timeout time.Duration, checkers ...Checker) *Service {
	return &Service{checkers: checkers, timeout: timeout}
}

func (s *Service) Liveness() Result {
	return Result{
		Status: StatusHealthy,
		Checks: map[string]string{"process": CheckUp},
	}
}

func (s *Service) Readiness(ctx context.Context) Result {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result := Result{Status: StatusHealthy, Checks: make(map[string]string, len(s.checkers))}
	var wg sync.WaitGroup
	var mutex sync.Mutex

	for _, checker := range s.checkers {
		checker := checker
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := CheckUp
			if err := checker.Check(ctx); err != nil {
				status = CheckDown
			}
			mutex.Lock()
			result.Checks[checker.Name()] = status
			mutex.Unlock()
		}()
	}

	wg.Wait()
	for _, status := range result.Checks {
		if status == CheckDown {
			result.Status = StatusDegraded
			break
		}
	}

	return result
}
