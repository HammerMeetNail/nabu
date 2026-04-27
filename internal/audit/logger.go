package audit

import (
	"context"
	"encoding/json"
	"log"
)

type Logger interface {
	Log(ctx context.Context, event string, attrs map[string]string)
}

type NopLogger struct{}

func (NopLogger) Log(context.Context, string, map[string]string) {}

type StdLogger struct {
	logger *log.Logger
}

func NewStdLogger(logger *log.Logger) StdLogger {
	if logger == nil {
		logger = log.Default()
	}
	return StdLogger{logger: logger}
}

func (l StdLogger) Log(_ context.Context, event string, attrs map[string]string) {
	payload := map[string]any{"event": event}
	for key, value := range attrs {
		payload[key] = value
	}
	encoded, _ := json.Marshal(payload)
	l.logger.Println(string(encoded))
}
