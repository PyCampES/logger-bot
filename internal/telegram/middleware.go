// Package telegram contains the bot's message handlers and middleware.
package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// ErrorReporter wraps a handler with a deferred recover() that turns panics
// into "Error: <msg>" replies. Mirrors the Python contract: the bot must
// not crash on a single bad message.
//
// Handlers should also return errors explicitly for known failure modes;
// this is the backstop for unexpected ones.
func ErrorReporter(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			slog.Error("handler panic", "panic", r)
			if update == nil || update.Message == nil || b == nil {
				return
			}
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("Error: %v", r),
			})
		}()
		next(ctx, b, update)
	}
}

// reply is a small convenience used by handlers to send a plain-text reply,
// swallowing the SendMessage error after logging it.
func reply(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		slog.Warn("send message failed", "err", err)
	}
}

// replyMarkdown sends a MarkdownV2 reply (used for JSON code blocks).
func replyMarkdown(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		slog.Warn("send markdown message failed", "err", err)
	}
}
