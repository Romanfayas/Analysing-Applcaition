package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TelegramNotifier sends alerts via Telegram bot API.
type TelegramNotifier struct {
	botToken string
	chatID   string
	client   *http.Client
}

// NewTelegramNotifier creates a Telegram notifier with the given credentials.
func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AlertType categorizes different alert types.
type AlertType string

const (
	AlertNewBuySignal        AlertType = "NEW_BUY_SIGNAL"
	AlertSignalChanged       AlertType = "SIGNAL_CHANGED"
	AlertStopLossReached     AlertType = "STOP_LOSS_REACHED"
	AlertTargetReached       AlertType = "TARGET_REACHED"
	AlertIPOOpens            AlertType = "IPO_OPENS"
	AlertIPOListed           AlertType = "IPO_LISTED"
	AlertShariahChanged      AlertType = "SHARIAH_STATUS_CHANGED"
	AlertDataQuality         AlertType = "DATA_QUALITY_ISSUE"
)

// Alert represents a notification to send.
type Alert struct {
	Type      AlertType `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Symbol    string    `json:"symbol,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SendAlert sends an alert via Telegram.
func (t *TelegramNotifier) SendAlert(alert Alert) error {
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram not configured: missing bot token or chat ID")
	}

	emoji := alertEmoji(alert.Type)
	text := fmt.Sprintf(
		"%s *%s*\n\n%s\n\n_%s_",
		emoji, escapeMarkdown(alert.Title),
		escapeMarkdown(alert.Message),
		alert.Timestamp.In(istLocation()).Format("02 Jan 2006, 03:04 PM IST"),
	)

	return t.sendMessage(text)
}

// sendMessage sends a raw message to the configured Telegram chat.
func (t *TelegramNotifier) sendMessage(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	payload := map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %w", err)
	}

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}

	return nil
}

// SendStartupNotification sends a message when the platform starts.
func (t *TelegramNotifier) SendStartupNotification() error {
	return t.SendAlert(Alert{
		Type:      "SYSTEM",
		Title:     "Halal Equity Platform Started",
		Message:   "The quantitative research platform is now online and ready for analysis.",
		Timestamp: time.Now(),
	})
}

// alertEmoji returns an emoji for each alert type.
func alertEmoji(alertType AlertType) string {
	switch alertType {
	case AlertNewBuySignal:
		return "🟢"
	case AlertSignalChanged:
		return "🔄"
	case AlertStopLossReached:
		return "🔴"
	case AlertTargetReached:
		return "🎯"
	case AlertIPOOpens:
		return "🆕"
	case AlertIPOListed:
		return "📈"
	case AlertShariahChanged:
		return "🛡️"
	case AlertDataQuality:
		return "⚠️"
	default:
		return "📋"
	}
}

// escapeMarkdown escapes special Markdown characters for Telegram.
func escapeMarkdown(text string) string {
	// For Telegram Markdown v1, escape _ * [ ] ( ) ~ ` > # + - = | { } . !
	replacer := map[string]string{
		"_": "\\_",
		"*": "\\*",
		"[": "\\[",
		"]": "\\]",
		"`": "\\`",
	}
	result := text
	for old, new := range replacer {
		result = replaceAll(result, old, new)
	}
	return result
}

func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if string(s[i]) == old {
			result += new
		} else {
			result += string(s[i])
		}
	}
	return result
}

// istLocation returns the IST timezone.
func istLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		// Fallback to UTC+5:30
		loc = time.FixedZone("IST", 5*60*60+30*60)
	}
	return loc
}
