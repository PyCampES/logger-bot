package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santiago-jauregui/logger-bot/internal/store"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// NewSQL returns the handler for `/sql <query>`.
//
// Safety: the underlying connection is read-only (`?mode=ro`). SQLite itself
// rejects writes; no SQL parsing or allowlisting is needed here.
func NewSQL(reader *store.Reader) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil {
			return
		}
		query := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/sql"))
		if query == "" {
			reply(ctx, b, msg.Chat.ID, "No query detected")
			return
		}

		rows, err := reader.Query(query)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		body, _ := json.MarshalIndent(rows, "", "  ")
		replyMarkdown(ctx, b, msg.Chat.ID, fmt.Sprintf("```\n%s\n```", body))
	}
}