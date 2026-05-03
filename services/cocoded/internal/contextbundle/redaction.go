package contextbundle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	redactionPlaceholder      = "[REDACTED]"
	defaultMinSecretValueSize = 8
)

type RedactionOptions struct {
	EnvValues       map[string]string
	MinSecretLength int
}

type RedactionReport struct {
	BundleID       string                `json:"bundle_id"`
	RedactionCount int                   `json:"redaction_count"`
	Items          []RedactionReportItem `json:"items"`
}

type RedactionReportItem struct {
	ItemID         string         `json:"item_id"`
	Kind           ItemKind       `json:"kind"`
	Path           string         `json:"path,omitempty"`
	Title          string         `json:"title,omitempty"`
	RedactionCount int            `json:"redaction_count"`
	Detectors      map[string]int `json:"detectors"`
}

type RedactionArtifactParams struct {
	ID              string
	WorkspaceID     string
	ReviewSessionID string
	BundleID        string
	CreatedAt       string
}

func RedactBundle(bundle Bundle, options RedactionOptions) (Bundle, RedactionReport, error) {
	items, report, err := RedactContextItems(bundle.ID, bundle.Items, options)
	if err != nil {
		return Bundle{}, RedactionReport{}, err
	}
	bundle.Items = items
	bundle = ApplyBundleTokenEstimates(bundle)
	if err := bundle.Validate(); err != nil {
		return Bundle{}, RedactionReport{}, err
	}
	return bundle, report, nil
}

func RedactContextItems(bundleID string, items []Item, options RedactionOptions) ([]Item, RedactionReport, error) {
	if strings.TrimSpace(bundleID) == "" {
		return nil, RedactionReport{}, errors.New("context bundle id is required")
	}
	options = normalizeRedactionOptions(options)
	redactors := buildSecretRedactors(options)

	out := make([]Item, len(items))
	report := RedactionReport{BundleID: bundleID}
	for index, item := range items {
		item = items[index]
		counts := map[string]int{}
		item.Content = redactString(item.Content, redactors, counts)
		item.Title = redactString(item.Title, redactors, counts)
		if len(item.Metadata) > 0 {
			metadata := redactString(string(item.Metadata), redactors, counts)
			if !json.Valid([]byte(metadata)) {
				return nil, RedactionReport{}, fmt.Errorf("redacted metadata for context item %q is invalid JSON", item.ID)
			}
			item.Metadata = json.RawMessage(metadata)
		}
		if len(counts) > 0 {
			item.TokenEstimate = EstimateItemTokens(item)
			reportItem := redactionReportItem(item, counts)
			report.Items = append(report.Items, reportItem)
			report.RedactionCount += reportItem.RedactionCount
		}
		if err := item.Validate(); err != nil {
			return nil, RedactionReport{}, err
		}
		out[index] = item
	}
	return out, report, nil
}

func SaveRedactionReportArtifact(ctx context.Context, store *artifact.Store, params RedactionArtifactParams, report RedactionReport) (dbgen.Artifact, error) {
	if store == nil {
		return dbgen.Artifact{}, errors.New("artifact store is required")
	}
	if strings.TrimSpace(params.ID) == "" {
		return dbgen.Artifact{}, errors.New("redaction report artifact id is required")
	}
	if strings.TrimSpace(params.WorkspaceID) == "" {
		return dbgen.Artifact{}, errors.New("workspace id is required")
	}
	bundleID := strings.TrimSpace(params.BundleID)
	if bundleID == "" {
		bundleID = report.BundleID
	}
	if bundleID == "" {
		return dbgen.Artifact{}, errors.New("context bundle id is required")
	}
	createdAt := strings.TrimSpace(params.CreatedAt)
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return dbgen.Artifact{}, fmt.Errorf("encode redaction report: %w", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"source":          "context_redaction",
		"bundle_id":       bundleID,
		"redaction_count": report.RedactionCount,
		"item_count":      len(report.Items),
	})
	if err != nil {
		return dbgen.Artifact{}, fmt.Errorf("encode redaction report metadata: %w", err)
	}
	return store.Save(ctx, artifact.SaveParams{
		ID:              params.ID,
		WorkspaceID:     params.WorkspaceID,
		ReviewSessionID: nullableRedactionReviewSession(params.ReviewSessionID),
		Kind:            "context_redaction_report",
		RelativePath:    fmt.Sprintf("context/%s/redaction-report.json", bundleID),
		ContentType:     "application/json",
		MetadataJSON:    string(metadata),
		CreatedAt:       createdAt,
	}, content)
}

func normalizeRedactionOptions(options RedactionOptions) RedactionOptions {
	if options.MinSecretLength <= 0 {
		options.MinSecretLength = defaultMinSecretValueSize
	}
	return options
}

type secretRedactor struct {
	name    string
	pattern *regexp.Regexp
	replace func(string) string
}

func buildSecretRedactors(options RedactionOptions) []secretRedactor {
	redactors := []secretRedactor{
		{
			name:    "private_key_block",
			pattern: regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`),
			replace: func(string) string {
				return redactionPlaceholder
			},
		},
		{
			name:    "bearer_token",
			pattern: regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{16,}`),
			replace: func(value string) string {
				parts := strings.Fields(value)
				if len(parts) > 0 {
					return parts[0] + " " + redactionPlaceholder
				}
				return redactionPlaceholder
			},
		},
		{
			name:    "openai_key",
			pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
			replace: func(string) string {
				return redactionPlaceholder
			},
		},
		{
			name:    "github_token",
			pattern: regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}\b`),
			replace: func(string) string {
				return redactionPlaceholder
			},
		},
		{
			name:    "jwt",
			pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
			replace: func(string) string {
				return redactionPlaceholder
			},
		},
		{
			name:    "secret_assignment",
			pattern: regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_KEY|ACCESS_KEY|PRIVATE_KEY)[A-Z0-9_]*\s*=\s*)(["']?)([^"'\s]{8,})(["']?)`),
			replace: redactSecretAssignment,
		},
	}
	for _, env := range sortedEnvSecrets(options.EnvValues, options.MinSecretLength) {
		pattern := regexp.MustCompile(regexp.QuoteMeta(env.value))
		name := "env_value:" + env.name
		redactors = append(redactors, secretRedactor{
			name:    name,
			pattern: pattern,
			replace: func(string) string {
				return redactionPlaceholder
			},
		})
	}
	return redactors
}

func redactString(value string, redactors []secretRedactor, counts map[string]int) string {
	for _, redactor := range redactors {
		value = redactor.pattern.ReplaceAllStringFunc(value, func(match string) string {
			counts[redactor.name]++
			return redactor.replace(match)
		})
	}
	return value
}

func redactSecretAssignment(value string) string {
	matches := regexp.MustCompile(`(?i)^([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_KEY|ACCESS_KEY|PRIVATE_KEY)[A-Z0-9_]*\s*=\s*)(["']?)([^"'\s]{8,})(["']?)$`).FindStringSubmatch(value)
	if len(matches) != 5 {
		return redactionPlaceholder
	}
	quote := matches[2]
	if quote == "" {
		quote = matches[4]
	}
	return matches[1] + quote + redactionPlaceholder + quote
}

type envSecret struct {
	name  string
	value string
}

func sortedEnvSecrets(values map[string]string, minLength int) []envSecret {
	secrets := make([]envSecret, 0, len(values))
	for name, value := range values {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || len(value) < minLength {
			continue
		}
		secrets = append(secrets, envSecret{name: name, value: value})
	}
	sort.Slice(secrets, func(i, j int) bool {
		if len(secrets[i].value) != len(secrets[j].value) {
			return len(secrets[i].value) > len(secrets[j].value)
		}
		return secrets[i].name < secrets[j].name
	})
	return secrets
}

func redactionReportItem(item Item, counts map[string]int) RedactionReportItem {
	detectors := make(map[string]int, len(counts))
	var total int
	for name, count := range counts {
		detectors[name] = count
		total += count
	}
	return RedactionReportItem{
		ItemID:         item.ID,
		Kind:           item.Kind,
		Path:           item.Path,
		Title:          item.Title,
		RedactionCount: total,
		Detectors:      detectors,
	}
}

func nullableRedactionReviewSession(reviewSessionID string) sql.NullString {
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	if reviewSessionID == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: reviewSessionID, Valid: true}
}
