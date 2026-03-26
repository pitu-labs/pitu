package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Poller struct {
	token   string
	baseURL string
	client  *http.Client
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
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
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
	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}
