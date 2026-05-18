package browser

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func CheckServer(ctx context.Context, targetURL string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("server %s tidak bisa diakses: %w", targetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("server %s merespons status %d", targetURL, resp.StatusCode)
	}
	return nil
}
