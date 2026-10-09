package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("log line is not JSON: %v\n%s", err, buf)
	}
	return m
}

func TestJSONUsesCloudLoggingFieldNames(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, FormatJSON).Warn("disk almost full")

	m := decode(t, &buf)
	if m["severity"] != "WARNING" {
		t.Errorf("severity = %v, want WARNING", m["severity"])
	}
	if m["message"] != "disk almost full" {
		t.Errorf("message = %v, want %q", m["message"], "disk almost full")
	}
	if _, ok := m["level"]; ok {
		t.Error("record still has a level field")
	}
}

func TestContextAttrsAreLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelInfo, FormatJSON).With("service", "api")

	ctx := WithAttrs(context.Background(), slog.String("request_id", "r-1"))
	ctx = WithAttrs(ctx, slog.String("document_id", "d-1"))
	logger.InfoContext(ctx, "uploaded")

	m := decode(t, &buf)
	for key, want := range map[string]string{"service": "api", "request_id": "r-1", "document_id": "d-1"} {
		if m[key] != want {
			t.Errorf("%s = %v, want %q", key, m[key], want)
		}
	}
}

func TestWithAttrsDoesNotShareState(t *testing.T) {
	base := WithAttrs(context.Background(), slog.String("a", "1"))
	first := WithAttrs(base, slog.String("b", "2"))
	_ = WithAttrs(base, slog.String("c", "3"))

	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, FormatJSON).InfoContext(first, "x")
	m := decode(t, &buf)
	if m["b"] != "2" {
		t.Errorf("b = %v, want 2", m["b"])
	}
	if _, ok := m["c"]; ok {
		t.Error("attribute from a sibling context leaked into the record")
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, slog.LevelWarn, FormatText).Info("hidden")
	if strings.Contains(buf.String(), "hidden") {
		t.Error("info record logged at warn level")
	}
}
