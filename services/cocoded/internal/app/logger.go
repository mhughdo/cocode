package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

func NewLogger(logPath string) (*slog.Logger, func(), error) {
	writers := []io.Writer{os.Stderr}
	var file *os.File

	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return nil, nil, err
		}

		opened, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, err
		}
		file = opened
		writers = append(writers, file)
	}

	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cleanup := func() {
		if file != nil {
			_ = file.Close()
		}
	}

	return logger, cleanup, nil
}
