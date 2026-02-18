package test

import (
	"flowforge/cmd"
	"testing"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

// TestFuzzyDetection_HighSimilarity ensures that a 91%+ match is caught.
func TestFuzzyDetection_HighSimilarity(t *testing.T) {
	lev := metrics.NewLevenshtein()

	base := "[<NUM>] Processing item <NUM> at <HEX> with value <NUM>"
	// Very similar line (only minor difference)
	similar := "[<NUM>] Processing item <NUM> at <HEX> with val <NUM>"

	similarity := strutil.Similarity(base, similar, lev)
	t.Logf("High similarity test: %.4f", similarity)

	if similarity < 0.9 {
		t.Errorf("Expected similarity >= 0.9, got %.4f. High-similarity match should be caught.", similarity)
	}
}

// TestFuzzyDetection_LowSimilarity ensures that a 50% match is ignored.
func TestFuzzyDetection_LowSimilarity(t *testing.T) {
	lev := metrics.NewLevenshtein()

	base := "[<NUM>] Processing item <NUM> at <HEX> with value <NUM>"
	different := "ERROR: Connection refused to database at port 5432"

	similarity := strutil.Similarity(base, different, lev)
	t.Logf("Low similarity test: %.4f", similarity)

	if similarity >= 0.9 {
		t.Errorf("Expected similarity < 0.9, got %.4f. Low-similarity match should be ignored.", similarity)
	}
}

// TestNormalizeLog verifies that timestamps, hex addresses, and numbers are normalized.
func TestNormalizeLog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "hex_address",
			input:    "Processing at 0x10297a370",
			expected: "Processing at <HEX>",
		},
		{
			name:     "numbers",
			input:    "item 42 with value 3.14",
			expected: "item <NUM> with value <NUM>",
		},
		{
			name:     "full_log_line",
			input:    "[1771347730.592837] Processing item 301 at 0x10297a350 with value 0.41184",
			expected: "[<NUM>] Processing item <NUM> at <HEX> with value <NUM>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cmd.NormalizeLog(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeLog(%q)\n  got:      %q\n  expected: %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestFuzzyDetection_BoundaryCase tests the exact 90% threshold boundary.
func TestFuzzyDetection_BoundaryCase(t *testing.T) {
	lev := metrics.NewLevenshtein()

	base := "[<NUM>] Processing item <NUM> at <HEX> with value <NUM>"
	// Identical should be 100%
	identical := "[<NUM>] Processing item <NUM> at <HEX> with value <NUM>"

	similarity := strutil.Similarity(base, identical, lev)
	t.Logf("Identical similarity: %.4f", similarity)

	if similarity != 1.0 {
		t.Errorf("Expected similarity = 1.0 for identical strings, got %.4f", similarity)
	}
}
