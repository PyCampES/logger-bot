package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/santiago-jauregui/logger-bot/internal/store"
	"github.com/santiago-jauregui/logger-bot/internal/telegram"
	"github.com/santiago-jauregui/logger-bot/internal/whisper"
	"github.com/go-telegram/bot"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required env var missing", "key", key)
		os.Exit(2)
	}
	return v
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	token := mustEnv("TELEGRAM_API_TOKEN")
	whisperAddr := envOr("WHISPER_ADDR", "whisper:50051")
	dbPath := envOr("DB_PATH", "/data/log.db")

	writer, err := store.NewWriter(dbPath)
	if err != nil {
		slog.Error("open writer", "err", err)
		os.Exit(1)
	}
	defer writer.Close()

	reader, err := store.NewReader(dbPath)
	if err != nil {
		slog.Error("open reader", "err", err)
		os.Exit(1)
	}
	defer reader.Close()

	wclient, err := whisper.NewClient(whisperAddr)
	if err != nil {
		slog.Error("dial whisper", "err", err)
		os.Exit(1)
	}
	defer wclient.Close()

	audioHandler := telegram.ErrorReporter(telegram.NewAudio(telegram.AudioDeps{
		Whisper: wclient,
		Writer:  writer,
	}))

	b, err := bot.New(token, bot.WithDefaultHandler(audioHandler))
	if err != nil {
		slog.Error("bot new", "err", err)
		os.Exit(1)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, "/last", bot.MatchTypePrefix,
		telegram.ErrorReporter(telegram.NewLast(reader)))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sql", bot.MatchTypePrefix,
		telegram.ErrorReporter(telegram.NewSQL(reader)))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/health", bot.MatchTypeExact,
		telegram.ErrorReporter(telegram.Health))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("Server up, you're ready to go", "whisper_addr", whisperAddr, "db_path", dbPath)
	b.Start(ctx)
	slog.Info("shutdown complete")
}
