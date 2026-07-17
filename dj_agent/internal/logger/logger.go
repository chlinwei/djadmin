package logger

import (
    "log/slog"
    "os"
    "strings"
)

func Init(level string) *slog.Logger {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: parseLevel(level),
    }))
    slog.SetDefault(logger)
    return logger
}

func parseLevel(v string) slog.Level {
    switch strings.ToLower(strings.TrimSpace(v)) {
    case "debug":
        return slog.LevelDebug
    case "warn":
        return slog.LevelWarn
    case "error":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}