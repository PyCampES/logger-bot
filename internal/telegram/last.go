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

// NewLast returns the handler for `/last <exercise>`.
//
// Empty result: replies "No previous workouts for <exercise>" — small UX
// improvement over the original which sent `null`.
func NewLast(reader *store.Reader) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil {
			return
		}
		args := commandArgs(msg.Text, "/last")
		switch len(args) {
		case 0:
			reply(ctx, b, msg.Chat.ID, "No exercise parsed")
			return
		case 1:
			// ok
		default:
			reply(ctx, b, msg.Chat.ID, "Only 1 arg is tolerated")
			return
		}
		exercise := args[0]

		row, err := reader.Last(exercise)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		if row == nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("No previous workouts for %s", exercise))
			return
		}
		body, _ := json.MarshalIndent(row, "", "  ")
		replyMarkdown(ctx, b, msg.Chat.ID, fmt.Sprintf("Last %s workout:\n```\n%s\n```", exercise, body))
	}
}

// commandArgs splits "/cmd a b c" -> ["a", "b", "c"].
func commandArgs(text, cmd string) []string {
	rest := strings.TrimSpace(strings.TrimPrefix(text, cmd))
	if rest == "" {
		return nil
	}
	return strings.Fields(rest)
}