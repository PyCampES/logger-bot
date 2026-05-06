package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/santiago-jauregui/logger-bot/internal/extraction"
	"github.com/santiago-jauregui/logger-bot/internal/store"
	"github.com/santiago-jauregui/logger-bot/internal/whisper"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AudioDeps wires the audio handler's two service dependencies.
type AudioDeps struct {
	Whisper *whisper.Client
	Writer  *store.Writer
}

// NewAudio returns the bot.HandlerFunc invoked for messages containing
// audio / voice / document. Mirrors the Python audio handler precedence:
// `audio or voice or document` (in that order).
func NewAudio(deps AudioDeps) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil {
			return
		}
		fileID, mime, ok := pickAudio(msg)
		if !ok {
			reply(ctx, b, msg.Chat.ID, "No audio file found.")
			return
		}

		audio, err := downloadFile(ctx, b, fileID)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		if len(audio) > 20*1024*1024 {
			reply(ctx, b, msg.Chat.ID, "Error: file too large (>20MB)")
			return
		}

		text, err := deps.Whisper.Transcribe(ctx, audio, mime)
		if err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", friendlyTranscribeErr(err)))
			return
		}

		workout := extraction.Parse(text)
		if err := deps.Writer.Write(workout); err != nil {
			reply(ctx, b, msg.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}

		body, _ := json.Marshal(workout)
		slog.Info("transcribed and logged", "exercise", workout.Exercise, "reps", workout.Reps)
		reply(ctx, b, msg.Chat.ID, "I heard:\n"+string(body))
	}
}

// pickAudio mirrors `audio = msg.audio or msg.voice or msg.document` from the
// Python handler — checked in that order. Returns (fileID, mimeType, ok).
func pickAudio(msg *models.Message) (string, string, bool) {
	if msg.Audio != nil {
		return msg.Audio.FileID, msg.Audio.MimeType, true
	}
	if msg.Voice != nil {
		return msg.Voice.FileID, msg.Voice.MimeType, true
	}
	if msg.Document != nil {
		return msg.Document.FileID, msg.Document.MimeType, true
	}
	return "", "", false
}

// downloadFile resolves a Telegram file_id to a download URL and returns the bytes.
func downloadFile(ctx context.Context, b *bot.Bot, fileID string) ([]byte, error) {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("getFile: %w", err)
	}
	url := b.FileDownloadLink(file)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	// 20MB cap as a hard backstop; Telegram's getFile already enforces this.
	return io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024+1))
}

// friendlyTranscribeErr converts gRPC status codes to user-facing messages.
func friendlyTranscribeErr(err error) string {
	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.InvalidArgument:
			return "could not decode audio"
		case codes.Internal:
			return "transcription failed"
		case codes.Unavailable:
			return "transcription service starting"
		case codes.DeadlineExceeded:
			return "transcription timed out"
		}
	}
	return err.Error()
}
