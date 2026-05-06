package telegram

import (
	"context"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// recordingHandler captures whether it was called and panics if asked to.
type recordingHandler struct {
	called bool
	panic  any
}

func (r *recordingHandler) handle(ctx context.Context, b *bot.Bot, u *models.Update) {
	r.called = true
	if r.panic != nil {
		panic(r.panic)
	}
}

func TestErrorReporter_PassesThroughOnSuccess(t *testing.T) {
	rec := &recordingHandler{}
	wrapped := ErrorReporter(rec.handle)
	wrapped(context.Background(), nil, &models.Update{
		Message: &models.Message{ID: 1, Chat: models.Chat{ID: 99}, Text: "hi"},
	})
	if !rec.called {
		t.Fatal("expected inner handler to be called")
	}
}

func TestErrorReporter_RecoversFromPanic(t *testing.T) {
	rec := &recordingHandler{panic: "boom"}
	wrapped := ErrorReporter(rec.handle)

	// Should not panic out of the middleware. (We pass nil bot; with no chat
	// available the middleware skips the SendMessage call but still recovers.)
	wrapped(context.Background(), nil, &models.Update{
		Message: nil,
	})
}
