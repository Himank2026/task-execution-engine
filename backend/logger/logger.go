package logger

import (
	"log/slog"
	"os"
)

// Init makes the global slog logger emit structured JSON to stdout. Structured
// logs (key=value pairs, not free-form strings) are machine-parseable, so a log
// aggregator can filter/search them in production. After this runs, any
// slog.Info/slog.Error call anywhere uses this handler.
func Init() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
}
