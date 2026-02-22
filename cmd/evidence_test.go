package cmd

import (
	"encoding/hex"
	"os"
	"testing"
)

func TestDecodeSigningKeyHexPrefix(t *testing.T) {
	key, err := decodeSigningKey("hex:00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("decodeSigningKey: %v", err)
	}
	if len(key) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(key))
	}
}

func TestDecodeSigningKeyAutoHex(t *testing.T) {
	raw := "00112233445566778899aabbccddeeff"
	key, err := decodeSigningKey(raw)
	if err != nil {
		t.Fatalf("decodeSigningKey: %v", err)
	}
	expected, _ := hex.DecodeString(raw)
	if string(key) != string(expected) {
		t.Fatalf("expected decoded hex key bytes")
	}
}

func TestResolveEvidenceSigningKeyFromEnv(t *testing.T) {
	old, had := os.LookupEnv("FLOWFORGE_EVIDENCE_SIGNING_KEY")
	if err := os.Setenv("FLOWFORGE_EVIDENCE_SIGNING_KEY", "0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("FLOWFORGE_EVIDENCE_SIGNING_KEY", old)
		} else {
			_ = os.Unsetenv("FLOWFORGE_EVIDENCE_SIGNING_KEY")
		}
	})

	key, err := resolveEvidenceSigningKey("")
	if err != nil {
		t.Fatalf("resolveEvidenceSigningKey: %v", err)
	}
	if len(key) != 16 {
		t.Fatalf("expected 16-byte key, got %d", len(key))
	}
}
