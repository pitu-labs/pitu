package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Sender struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewSender(token, baseURL string) *Sender {
	return &Sender{token: token, baseURL: baseURL, client: &http.Client{}}
}

func (s *Sender) SendMessage(chatID, text string) error {
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	url := fmt.Sprintf("%s/bot%s/sendMessage", s.baseURL, s.token)
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: sendMessage: %w", err)
	}
	defer resp.Body.Close()
	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("telegram: decode response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram: API error: %s", result.Description)
	}
	return nil
}
