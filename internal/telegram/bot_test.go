package telegram

import (
	"testing"
	"time"
)

func TestNewBot_EmptyToken(t *testing.T) {
	bot, err := NewBot("", "123456")
	if err != nil {
		t.Fatalf("expected no error for empty token, got: %v", err)
	}
	if bot == nil {
		t.Fatal("expected bot to be non-nil")
	}
	if !bot.disabled {
		t.Error("expected bot to be disabled when token is empty")
	}
}

func TestNewBot_InvalidChatID(t *testing.T) {
	_, err := NewBot("fake-token", "not-a-number")
	if err == nil {
		t.Fatal("expected error for invalid chat ID")
	}
}

func TestBot_DisabledMode_SendMessage(t *testing.T) {
	bot := &Bot{disabled: true}

	err := bot.SendMessage("test message")
	if err != nil {
		t.Errorf("expected no error from disabled bot, got: %v", err)
	}
}

func TestBot_DisabledMode_SendAlert(t *testing.T) {
	bot := &Bot{disabled: true}

	err := bot.SendAlert("Test Title", "test body")
	if err != nil {
		t.Errorf("expected no error from disabled bot, got: %v", err)
	}
}

func TestBot_DisabledMode_AllNotifications(t *testing.T) {
	bot := &Bot{disabled: true}

	tests := []struct {
		name string
		fn   func() error
	}{
		{"NotifyStarted", bot.NotifyStarted},
		{"NotifyStopped", bot.NotifyStopped},
		{"NotifyMarketFound", func() error {
			return bot.NotifyMarketFound("test-market", time.Now().Add(time.Hour))
		}},
		{"NotifyOrderExecuted", func() error {
			return bot.NotifyOrderExecuted("BUY", 0.99, 100.0, 1.0)
		}},
		{"NotifyError", func() error {
			return bot.NotifyError(errTest)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

var errTest = testError{}

type testError struct{}

func (testError) Error() string { return "test error" }

func TestBot_SetDryRun(t *testing.T) {
	bot := &Bot{disabled: true}

	bot.SetDryRun(true)
	if !bot.dryRun {
		t.Error("expected dryRun to be true")
	}

	bot.SetDryRun(false)
	if bot.dryRun {
		t.Error("expected dryRun to be false")
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"plain text", "plain text"},
		{"*bold*", "\\*bold\\*"},
		{"_italic_", "\\_italic\\_"},
		{"`code`", "\\`code\\`"},
		{"[link](url)", "\\[link\\]\\(url\\)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("escapeMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{-1 * time.Second, "ended"},
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m 30s"},
		{3661 * time.Second, "1h 1m 1s"},
		{7200 * time.Second, "2h 0m 0s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}
