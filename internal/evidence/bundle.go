package evidence

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flowforge/internal/database"
)

const (
	ManifestFilename  = "manifest.json"
	SignatureFilename = "signature.json"
)

type ExportOptions struct {
	OutDir        string
	IncidentID    string
	TimelineLimit int
	AuditLimit    int
	DecisionLimit int
	ChainLimit    int
}

type BundleFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Manifest struct {
	Version            string       `json:"version"`
	BundleID           string       `json:"bundle_id"`
	GeneratedAt        string       `json:"generated_at"`
	SourceDBPath       string       `json:"source_db_path"`
	SelectedIncidentID string       `json:"selected_incident_id,omitempty"`
	Files              []BundleFile `json:"files"`
}

type Signature struct {
	Algorithm      string `json:"algorithm"`
	KeyID          string `json:"key_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Signature      string `json:"signature"`
}

type ExportResult struct {
	BundleDir string
	Manifest  Manifest
	Signature Signature
}

type VerifyResult struct {
	BundleDir   string
	FileCount   int
	ManifestOK  bool
	SignatureOK bool
}

type bundleSummary struct {
	GeneratedAt        string `json:"generated_at"`
	IncidentCount      int    `json:"incident_count"`
	TimelineEventCount int    `json:"timeline_event_count"`
	AuditEventCount    int    `json:"audit_event_count"`
	DecisionCount      int    `json:"decision_count"`
	IncidentChainCount int    `json:"incident_chain_count,omitempty"`
	SelectedIncidentID string `json:"selected_incident_id,omitempty"`
}

func Export(opts ExportOptions, signingKey []byte) (ExportResult, error) {
	if len(signingKey) == 0 {
		return ExportResult{}, errors.New("signing key is required")
	}
	if opts.OutDir == "" {
		return ExportResult{}, errors.New("output directory is required")
	}
	if opts.TimelineLimit <= 0 {
		opts.TimelineLimit = 500
	}
	if opts.AuditLimit <= 0 {
		opts.AuditLimit = 500
	}
	if opts.DecisionLimit <= 0 {
		opts.DecisionLimit = 500
	}
	if opts.ChainLimit <= 0 {
		opts.ChainLimit = 500
	}

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("create bundle directory: %w", err)
	}

	incidents, err := database.GetAllIncidents()
	if err != nil {
		return ExportResult{}, fmt.Errorf("load incidents: %w", err)
	}
	timeline, err := database.GetTimeline(opts.TimelineLimit)
	if err != nil {
		return ExportResult{}, fmt.Errorf("load timeline: %w", err)
	}
	audits, err := database.GetAuditEvents(opts.AuditLimit)
	if err != nil {
		return ExportResult{}, fmt.Errorf("load audit events: %w", err)
	}
	decisions, err := database.GetDecisionTraces(opts.DecisionLimit)
	if err != nil {
		return ExportResult{}, fmt.Errorf("load decision traces: %w", err)
	}

	chain := make([]database.UnifiedEvent, 0)
	if strings.TrimSpace(opts.IncidentID) != "" {
		chain, err = database.GetIncidentTimelineByIncidentID(strings.TrimSpace(opts.IncidentID), opts.ChainLimit)
		if err != nil {
			return ExportResult{}, fmt.Errorf("load incident chain: %w", err)
		}
	}

	files := make([]BundleFile, 0, 8)
	record := func(name string, v any) error {
		f, err := writeJSONPayload(opts.OutDir, name, v)
		if err != nil {
			return err
		}
		files = append(files, f)
		return nil
	}

	now := time.Now().UTC()
	generatedAt := now.Format(time.RFC3339)
	if err := record("incidents.json", incidents); err != nil {
		return ExportResult{}, err
	}
	if err := record("timeline.json", timeline); err != nil {
		return ExportResult{}, err
	}
	if err := record("audit_events.json", audits); err != nil {
		return ExportResult{}, err
	}
	if err := record("decision_traces.json", decisions); err != nil {
		return ExportResult{}, err
	}

	summary := bundleSummary{
		GeneratedAt:        generatedAt,
		IncidentCount:      len(incidents),
		TimelineEventCount: len(timeline),
		AuditEventCount:    len(audits),
		DecisionCount:      len(decisions),
		IncidentChainCount: len(chain),
		SelectedIncidentID: strings.TrimSpace(opts.IncidentID),
	}
	if err := record("summary.json", summary); err != nil {
		return ExportResult{}, err
	}
	if strings.TrimSpace(opts.IncidentID) != "" {
		if err := record("incident_chain.json", chain); err != nil {
			return ExportResult{}, err
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	manifest := Manifest{
		Version:            "v1",
		BundleID:           "evidence-" + now.Format("20060102-150405"),
		GeneratedAt:        generatedAt,
		SourceDBPath:       resolveSourceDBPath(),
		SelectedIncidentID: strings.TrimSpace(opts.IncidentID),
		Files:              files,
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ExportResult{}, fmt.Errorf("marshal manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	manifestPath := filepath.Join(opts.OutDir, ManifestFilename)
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return ExportResult{}, fmt.Errorf("write manifest: %w", err)
	}

	manifestDigest := sha256.Sum256(manifestBytes)
	signature := Signature{
		Algorithm:      "HMAC-SHA256",
		KeyID:          keyID(signingKey),
		ManifestSHA256: hex.EncodeToString(manifestDigest[:]),
		Signature:      sign(manifestBytes, signingKey),
	}
	signatureBytes, err := json.MarshalIndent(signature, "", "  ")
	if err != nil {
		return ExportResult{}, fmt.Errorf("marshal signature: %w", err)
	}
	signatureBytes = append(signatureBytes, '\n')
	signaturePath := filepath.Join(opts.OutDir, SignatureFilename)
	if err := os.WriteFile(signaturePath, signatureBytes, 0o644); err != nil {
		return ExportResult{}, fmt.Errorf("write signature: %w", err)
	}

	return ExportResult{
		BundleDir: opts.OutDir,
		Manifest:  manifest,
		Signature: signature,
	}, nil
}

func Verify(bundleDir string, signingKey []byte) (VerifyResult, error) {
	if bundleDir == "" {
		return VerifyResult{}, errors.New("bundle directory is required")
	}
	if len(signingKey) == 0 {
		return VerifyResult{}, errors.New("signing key is required")
	}

	manifestPath := filepath.Join(bundleDir, ManifestFilename)
	signaturePath := filepath.Join(bundleDir, SignatureFilename)

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("read manifest: %w", err)
	}
	signatureBytes, err := os.ReadFile(signaturePath)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("read signature: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return VerifyResult{}, fmt.Errorf("decode manifest: %w", err)
	}
	var sig Signature
	if err := json.Unmarshal(signatureBytes, &sig); err != nil {
		return VerifyResult{}, fmt.Errorf("decode signature: %w", err)
	}

	manifestDigest := sha256.Sum256(manifestBytes)
	manifestDigestHex := hex.EncodeToString(manifestDigest[:])
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(manifestDigestHex)), []byte(strings.ToLower(sig.ManifestSHA256))) != 1 {
		return VerifyResult{}, fmt.Errorf("manifest digest mismatch")
	}

	expectedSig := sign(manifestBytes, signingKey)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(expectedSig)), []byte(strings.ToLower(sig.Signature))) != 1 {
		return VerifyResult{}, fmt.Errorf("signature mismatch")
	}

	for _, f := range manifest.Files {
		if strings.TrimSpace(f.Path) == "" {
			return VerifyResult{}, fmt.Errorf("manifest file entry has empty path")
		}
		actual, err := fileDigest(filepath.Join(bundleDir, f.Path))
		if err != nil {
			return VerifyResult{}, fmt.Errorf("file digest %s: %w", f.Path, err)
		}
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(actual.SHA256)), []byte(strings.ToLower(f.SHA256))) != 1 {
			return VerifyResult{}, fmt.Errorf("file digest mismatch: %s", f.Path)
		}
		if actual.Bytes != f.Bytes {
			return VerifyResult{}, fmt.Errorf("file size mismatch: %s", f.Path)
		}
	}

	return VerifyResult{
		BundleDir:   bundleDir,
		FileCount:   len(manifest.Files),
		ManifestOK:  true,
		SignatureOK: true,
	}, nil
}

func writeJSONPayload(outDir, filename string, payload any) (BundleFile, error) {
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return BundleFile{}, fmt.Errorf("marshal %s: %w", filename, err)
	}
	b = append(b, '\n')
	fullPath := filepath.Join(outDir, filename)
	if err := os.WriteFile(fullPath, b, 0o644); err != nil {
		return BundleFile{}, fmt.Errorf("write %s: %w", filename, err)
	}
	return fileDigest(fullPath)
}

func fileDigest(path string) (BundleFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return BundleFile{}, err
	}
	sum := sha256.Sum256(b)
	return BundleFile{
		Path:   filepath.Base(path),
		SHA256: hex.EncodeToString(sum[:]),
		Bytes:  int64(len(b)),
	}, nil
}

func keyID(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:8])
}

func sign(payload []byte, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func resolveSourceDBPath() string {
	dbPath := os.Getenv("FLOWFORGE_DB_PATH")
	if strings.TrimSpace(dbPath) == "" {
		return "flowforge.db"
	}
	return dbPath
}
