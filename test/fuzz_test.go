package test

import (
	"agent-sentry/cmd"
	"agent-sentry/internal/redact"
	"testing"
)

func FuzzNormalizeLog(f *testing.F) {
	seeds := []string{
		"[1771347730.592837] Processing item 301 at 0x10297a350 with value 0.41184",
		"token=abc123 secret=shhh",
		"2026-02-17T12:00:00Z worker=42 addr=0xdeadbeef",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_ = cmd.NormalizeLog(input)
	})
}

func FuzzRedactLine(f *testing.F) {
	seeds := []string{
		"Authorization: Bearer abc.def.ghi",
		"SENTRY_API_KEY=abcd1234",
		"AKIAIOSFODNN7EXAMPLE",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_ = redact.Line(input)
	})
}
