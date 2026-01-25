package telegram

import (
	"fmt"
	"log"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot handles Telegram notifications for the sniper bot.
type Bot struct {
	api      *tgbotapi.BotAPI
	chatID   int64
	dryRun   bool
	disabled bool
}

// NewBot creates a new Telegram bot instance.
// If token is empty, returns a no-op bot that logs messages instead of sending.
func NewBot(token, chatID string) (*Bot, error) {
	if token == "" {
		log.Println("[telegram] no token provided, running in disabled mode (logging only)")
		return &Bot{disabled: true}, nil
	}

	parsedChatID, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID %q: %w", chatID, err)
	}

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	log.Printf("[telegram] authorized as @%s", api.Self.UserName)

	return &Bot{
		api:    api,
		chatID: parsedChatID,
	}, nil
}

// SetDryRun sets the dry run mode flag for notifications.
func (b *Bot) SetDryRun(dryRun bool) {
	b.dryRun = dryRun
}

// SendMessage sends a plain text message.
func (b *Bot) SendMessage(text string) error {
	return b.send(text, false)
}

// SendAlert sends a formatted alert with bold title.
func (b *Bot) SendAlert(title, message string) error {
	formatted := fmt.Sprintf("*%s*\n\n%s", escapeMarkdown(title), message)
	return b.send(formatted, true)
}

// NotifyStarted sends a notification that the bot has started.
func (b *Bot) NotifyStarted() error {
	mode := "LIVE"
	if b.dryRun {
		mode = "DRY_RUN"
	}
	return b.SendAlert("Bot Started", fmt.Sprintf("Polymarket Sniper is running in `%s` mode", mode))
}

// NotifyStopped sends a notification that the bot has stopped.
func (b *Bot) NotifyStopped() error {
	return b.SendAlert("Bot Stopped", "Polymarket Sniper has been shut down")
}

// NotifyMarketFound sends a notification when a market is found.
func (b *Bot) NotifyMarketFound(market string, endTime time.Time) error {
	timeUntilEnd := time.Until(endTime)
	return b.SendAlert("Market Found",
		fmt.Sprintf("Market: `%s`\nEnds: `%s`\nTime until end: `%s`",
			market,
			endTime.Format(time.RFC3339),
			formatDuration(timeUntilEnd),
		),
	)
}

// NotifyOrderExecuted sends a notification when an order is executed.
func (b *Bot) NotifyOrderExecuted(side string, price, size, profit float64) error {
	return b.SendAlert("Order Executed",
		fmt.Sprintf("Side: `%s`\nPrice: `%.4f`\nSize: `%.2f`\nExpected Profit: `$%.2f`",
			side, price, size, profit,
		),
	)
}

// NotifyError sends an error notification.
func (b *Bot) NotifyError(err error) error {
	return b.SendAlert("Error", fmt.Sprintf("`%s`", err.Error()))
}

// send handles the actual message sending with graceful error handling.
func (b *Bot) send(text string, useMarkdown bool) error {
	if b.disabled {
		log.Printf("[telegram] (disabled) %s", text)
		return nil
	}

	msg := tgbotapi.NewMessage(b.chatID, text)
	if useMarkdown {
		msg.ParseMode = tgbotapi.ModeMarkdown
	}

	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[telegram] failed to send message: %v", err)
		return fmt.Errorf("telegram send failed: %w", err)
	}

	return nil
}

// escapeMarkdown escapes special Markdown characters in text.
func escapeMarkdown(text string) string {
	replacer := []string{
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	}

	result := text
	for i := 0; i < len(replacer); i += 2 {
		result = replaceAll(result, replacer[i], replacer[i+1])
	}
	return result
}

// replaceAll replaces all occurrences of old with new in s.
func replaceAll(s, old, new string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result = append(result, new...)
			i += len(old) - 1
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}

// formatDuration formats a duration in a human-readable format.
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "ended"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
