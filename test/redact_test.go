package test

import (
	"agent-sentry/internal/redact"
	"strings"
	"testing"
)

func TestRedactLine(t *testing.T) {
	input := "Authorization: Bearer secret-token SENTRY_API_KEY=supersecret"
	out := redact.Line(input)
	if strings.Contains(out, "secret-token") || strings.Contains(out, "supersecret") {
		t.Fatalf("expected secrets to be redacted, got %q", out)
	}
}
