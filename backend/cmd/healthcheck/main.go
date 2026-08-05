package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8080/health/ready", nil)
	if err != nil {
		fail(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fail(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		fail(fmt.Errorf("unexpected status %d", response.StatusCode))
	}
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
