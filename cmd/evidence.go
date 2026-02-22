package cmd

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flowforge/internal/database"
	"flowforge/internal/evidence"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	evidenceOutDir        string
	evidenceIncidentID    string
	evidenceTimelineLimit int
	evidenceAuditLimit    int
	evidenceDecisionLimit int
	evidenceChainLimit    int
	evidenceSigningKeyRaw string
	evidenceVerifyDir     string
	hexKeyPattern         = regexp.MustCompile(`^[0-9a-fA-F]+$`)
)

var evidenceCmd = &cobra.Command{
	Use:   "evidence",
	Short: "Manage signed evidence bundles",
	Long:  "Export and verify signed evidence bundles for incidents, timeline, and audit trails.",
}

var evidenceExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export a signed evidence bundle",
	Long: `Exports evidence JSON artifacts and writes manifest + HMAC signature.

Signing key resolution order:
1. --key
2. FLOWFORGE_EVIDENCE_SIGNING_KEY
3. FLOWFORGE_MASTER_KEY`,
	Run: func(cmd *cobra.Command, args []string) {
		runEvidenceExport()
	},
}

var evidenceVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify a signed evidence bundle",
	Long: `Verifies:
1. manifest hash integrity
2. signature validity (HMAC-SHA256)
3. file hash and size for all manifest entries`,
	Run: func(cmd *cobra.Command, args []string) {
		runEvidenceVerify()
	},
}

func init() {
	rootCmd.AddCommand(evidenceCmd)
	evidenceCmd.AddCommand(evidenceExportCmd)
	evidenceCmd.AddCommand(evidenceVerifyCmd)

	evidenceExportCmd.Flags().StringVar(&evidenceOutDir, "out-dir", "", "Output directory for evidence bundle (default pilot_artifacts/evidence-<timestamp>)")
	evidenceExportCmd.Flags().StringVar(&evidenceIncidentID, "incident-id", "", "Optional incident ID for incident_chain.json export")
	evidenceExportCmd.Flags().IntVar(&evidenceTimelineLimit, "timeline-limit", 500, "Timeline event export limit")
	evidenceExportCmd.Flags().IntVar(&evidenceAuditLimit, "audit-limit", 500, "Audit event export limit")
	evidenceExportCmd.Flags().IntVar(&evidenceDecisionLimit, "decision-limit", 500, "Decision trace export limit")
	evidenceExportCmd.Flags().IntVar(&evidenceChainLimit, "chain-limit", 500, "Incident chain export limit")
	evidenceExportCmd.Flags().StringVar(&evidenceSigningKeyRaw, "key", "", "Signing key override (supports plain, hex:<key>, base64:<key>)")

	evidenceVerifyCmd.Flags().StringVar(&evidenceVerifyDir, "bundle-dir", "", "Evidence bundle directory to verify")
	evidenceVerifyCmd.Flags().StringVar(&evidenceSigningKeyRaw, "key", "", "Signing key override (supports plain, hex:<key>, base64:<key>)")
	_ = evidenceVerifyCmd.MarkFlagRequired("bundle-dir")
}

func runEvidenceExport() {
	if err := database.InitDB(); err != nil {
		fmt.Printf("Error: failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer database.CloseDB()

	key, err := resolveEvidenceSigningKey(evidenceSigningKeyRaw)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	outDir := strings.TrimSpace(evidenceOutDir)
	if outDir == "" {
		outDir = fmt.Sprintf("pilot_artifacts/evidence-%s", time.Now().Format("20060102-150405"))
	}

	result, err := evidence.Export(evidence.ExportOptions{
		OutDir:        outDir,
		IncidentID:    strings.TrimSpace(evidenceIncidentID),
		TimelineLimit: evidenceTimelineLimit,
		AuditLimit:    evidenceAuditLimit,
		DecisionLimit: evidenceDecisionLimit,
		ChainLimit:    evidenceChainLimit,
	}, key)
	if err != nil {
		fmt.Printf("Error: evidence export failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Evidence bundle exported: %s\n", result.BundleDir)
	fmt.Printf("Manifest: %s\n", filepathJoin(result.BundleDir, evidence.ManifestFilename))
	fmt.Printf("Signature: %s\n", filepathJoin(result.BundleDir, evidence.SignatureFilename))
	fmt.Printf("Signed files: %d\n", len(result.Manifest.Files))
}

func runEvidenceVerify() {
	key, err := resolveEvidenceSigningKey(evidenceSigningKeyRaw)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	dir := strings.TrimSpace(evidenceVerifyDir)
	if dir == "" {
		fmt.Println("Error: --bundle-dir is required")
		os.Exit(1)
	}

	result, err := evidence.Verify(dir, key)
	if err != nil {
		fmt.Printf("Error: evidence verify failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Evidence bundle verified: %s\n", result.BundleDir)
	fmt.Printf("File integrity checks: %d\n", result.FileCount)
	fmt.Printf("Manifest: PASS\n")
	fmt.Printf("Signature: PASS\n")
}

func resolveEvidenceSigningKey(rawFlag string) ([]byte, error) {
	raw := strings.TrimSpace(rawFlag)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("FLOWFORGE_EVIDENCE_SIGNING_KEY"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("FLOWFORGE_MASTER_KEY"))
	}
	if raw == "" {
		return nil, errors.New("signing key missing (set --key or FLOWFORGE_EVIDENCE_SIGNING_KEY or FLOWFORGE_MASTER_KEY)")
	}

	key, err := decodeSigningKey(raw)
	if err != nil {
		return nil, err
	}
	if len(key) < 16 {
		return nil, errors.New("signing key must be at least 16 bytes after decoding")
	}
	return key, nil
}

func decodeSigningKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "hex:"):
		b, err := hex.DecodeString(strings.TrimPrefix(raw, "hex:"))
		if err != nil {
			return nil, fmt.Errorf("invalid hex signing key: %w", err)
		}
		return b, nil
	case strings.HasPrefix(raw, "base64:"):
		b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "base64:"))
		if err != nil {
			return nil, fmt.Errorf("invalid base64 signing key: %w", err)
		}
		return b, nil
	default:
		if looksLikeHex(raw) {
			if b, err := hex.DecodeString(raw); err == nil {
				return b, nil
			}
		}
		return []byte(raw), nil
	}
}

func looksLikeHex(s string) bool {
	if len(s)%2 != 0 || len(s) == 0 {
		return false
	}
	return hexKeyPattern.MatchString(s)
}

func filepathJoin(dir, name string) string {
	return filepath.Join(dir, name)
}
