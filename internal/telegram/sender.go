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

// SendChatAction posts a transient status indicator (e.g. "typing") to the chat.
// The indicator disappears after ~5 s or when the bot sends a message.
func (s *Sender) SendChatAction(chatID, action string) error {
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "action": action})
	url := fmt.Sprintf("%s/bot%s/sendChatAction", s.baseURL, s.token)
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: sendChatAction: %w", err)
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

// ReactToMessage sets an emoji reaction on a specific message.
// messageID is the Telegram integer message_id. emoji is a single Unicode emoji string.
func (s *Sender) ReactToMessage(chatID string, messageID int, emoji string) error {
	body, _ := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
	})
	url := fmt.Sprintf("%s/bot%s/setMessageReaction", s.baseURL, s.token)
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: setMessageReaction: %w", err)
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
