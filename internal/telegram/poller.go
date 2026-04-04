package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type Poller struct {
	token   string
	baseURL string
	client  *http.Client
}

// retryError is returned by getUpdates when the server signals a back-off.
// The after field carries the duration from the Retry-After header (or a default).
type retryError struct {
	after time.Duration
}

func (e *retryError) Error() string {
	return fmt.Sprintf("telegram: rate limited; retry after %s", e.after)
}

// parseRetryAfter converts a Retry-After header value (integer seconds) to a
// Duration. Returns 10 s on empty, negative, or non-integer values.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 10 * time.Second
	}
	n, err := strconv.Atoi(header)
	if err != nil || n < 0 {
		return 10 * time.Second
	}
	return time.Duration(n) * time.Second
}

func NewPoller(token, baseURL string) *Poller {
	return &Poller{
		token:   token,
		baseURL: fmt.Sprintf("%s/bot%s", baseURL, token),
		client:  &http.Client{Timeout: 35 * time.Second},
	}
}

// Poll calls handler for each received Update until ctx is cancelled.
func (p *Poller) Poll(ctx context.Context, handler func(Update)) {
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		updates, err := p.getUpdates(ctx, offset, 30)
		if err != nil {
			var re *retryError
			backoff := 2 * time.Second
			if errors.As(err, &re) {
				backoff = re.after
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				continue
			}
		}
		for _, u := range updates {
			handler(u)
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

func (p *Poller) getUpdates(ctx context.Context, offset, timeout int) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", p.baseURL, offset, timeout)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &retryError{after: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("telegram: server error %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}
