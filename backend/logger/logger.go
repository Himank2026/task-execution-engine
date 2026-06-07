package logger

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// Init configures the global slog logger. After this runs, any slog.Info/slog.Error
// call anywhere uses this handler.
//
// The format is chosen by the LOG_FORMAT env var:
//   - "json" (default): one JSON object per line — machine-parseable, what you want
//     in production where a log aggregator ingests and indexes it.
//   - "text": human-friendly "key=value" lines with a short HH:MM:SS time — much
//     easier to read while developing. Run with LOG_FORMAT=text for this.
//
// Structured logging either way: every field is a key=value pair, not baked into a
// free-form string, so logs stay searchable/filterable.
func Init() {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	var handler slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "text" {
		// In text mode, shorten the timestamp to just the clock time so lines are easy
		// to scan. (Full RFC3339 timestamps are great for machines, noisy for humans.)
		opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format("15:04:05"))
				}
			}
			return a
		}
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
