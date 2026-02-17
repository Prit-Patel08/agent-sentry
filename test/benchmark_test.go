package test

import (
	"agent-sentry/cmd"
	"agent-sentry/internal/redact"
	"testing"
)

func BenchmarkNormalizeLog(b *testing.B) {
	line := "[1771347730.592837] Processing item 301 at 0x10297a350 with value 0.41184"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cmd.NormalizeLog(line)
	}
}

func BenchmarkRedactLine(b *testing.B) {
	line := "Authorization: Bearer secret-token-value SENTRY_API_KEY=abcd1234ef567890abcdef1234567890"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = redact.Line(line)
	}
}
