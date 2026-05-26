package contextbundle

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

type Scope string

const (
	ScopeReview      Scope = "review"
	ScopeFinding     Scope = "finding"
	ScopeEvidenceMap Scope = "evidence_map"
	ScopeFollowUp    Scope = "follow_up"
)

type ItemKind string

const (
	ItemChangedHunk    ItemKind = "changed_hunk"
	ItemFileSlice      ItemKind = "file_slice"
	ItemFullFile       ItemKind = "full_file"
	ItemRelatedCode    ItemKind = "related_code"
	ItemRelatedTest    ItemKind = "related_test"
	ItemProjectRule    ItemKind = "project_rule"
	ItemPriorComment   ItemKind = "prior_comment"
	ItemPriorDecision  ItemKind = "prior_decision"
	ItemEvidence       ItemKind = "evidence"
	ItemFocusFile      ItemKind = "focus_file"
	ItemRedactionNote  ItemKind = "redaction_note"
	ItemPromptMaterial ItemKind = "prompt_material"
)

type Bundle struct {
	ID              string          `json:"id"`
	ReviewSessionID string          `json:"review_session_id"`
	AgentConfigID   string          `json:"agent_config_id,omitempty"`
	Scope           Scope           `json:"scope"`
	TokenEstimate   int64           `json:"token_estimate"`
	ItemCount       int64           `json:"item_count"`
	ArtifactID      string          `json:"artifact_id,omitempty"`
	Policy          json.RawMessage `json:"policy"`
	CreatedAt       string          `json:"created_at"`
	Items           []Item          `json:"items,omitempty"`
}

type Item struct {
	ID                string          `json:"id"`
	ContextBundleID   string          `json:"context_bundle_id"`
	Kind              ItemKind        `json:"kind"`
	Path              string          `json:"path,omitempty"`
	StartLine         int64           `json:"start_line,omitempty"`
	EndLine           int64           `json:"end_line,omitempty"`
	Title             string          `json:"title,omitempty"`
	Content           string          `json:"content,omitempty"`
	ContentArtifactID string          `json:"content_artifact_id,omitempty"`
	TokenEstimate     int64           `json:"token_estimate"`
	Metadata          json.RawMessage `json:"metadata"`
}

func (s Scope) Valid() bool {
	switch s {
	case ScopeReview, ScopeFinding, ScopeEvidenceMap, ScopeFollowUp:
		return true
	default:
		return false
	}
}

func (b Bundle) Validate() error {
	if strings.TrimSpace(b.ID) == "" {
		return errors.New("context bundle id is required")
	}
	if strings.TrimSpace(b.ReviewSessionID) == "" {
		return errors.New("review session id is required")
	}
	if !b.Scope.Valid() {
		return fmt.Errorf("context bundle scope %q is invalid", b.Scope)
	}
	if b.TokenEstimate < 0 {
		return errors.New("context bundle token estimate cannot be negative")
	}
	if b.ItemCount < 0 {
		return errors.New("context bundle item count cannot be negative")
	}
	if !validRawJSON(b.Policy) {
		return errors.New("context bundle policy must be valid JSON")
	}
	for _, item := range b.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if item.ContextBundleID != b.ID {
			return fmt.Errorf("context item %q belongs to bundle %q, want %q", item.ID, item.ContextBundleID, b.ID)
		}
	}
	return nil
}

func (i Item) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return errors.New("context item id is required")
	}
	if strings.TrimSpace(i.ContextBundleID) == "" {
		return errors.New("context item bundle id is required")
	}
	if strings.TrimSpace(string(i.Kind)) == "" {
		return errors.New("context item kind is required")
	}
	if i.TokenEstimate < 0 {
		return errors.New("context item token estimate cannot be negative")
	}
	if (i.StartLine == 0) != (i.EndLine == 0) {
		return errors.New("context item line range must include both start and end lines")
	}
	if i.StartLine < 0 || i.EndLine < 0 || (i.StartLine > 0 && i.StartLine > i.EndLine) {
		return errors.New("context item line range is invalid")
	}
	if !validRawJSON(i.Metadata) {
		return errors.New("context item metadata must be valid JSON")
	}
	return nil
}

func BundleFromRows(row dbgen.ContextBundle, itemRows []dbgen.ContextItem) (Bundle, error) {
	policy, err := decodeJSONField("context bundle policy", row.PolicyJson)
	if err != nil {
		return Bundle{}, err
	}
	items := make([]Item, 0, len(itemRows))
	for _, itemRow := range itemRows {
		item, err := ItemFromRow(itemRow)
		if err != nil {
			return Bundle{}, err
		}
		items = append(items, item)
	}
	bundle := Bundle{
		ID:              row.ID,
		ReviewSessionID: row.ReviewSessionID,
		AgentConfigID:   nullableString(row.AgentConfigID),
		Scope:           Scope(row.Scope),
		TokenEstimate:   row.TokenEstimate,
		ItemCount:       row.ItemCount,
		ArtifactID:      nullableString(row.ArtifactID),
		Policy:          policy,
		CreatedAt:       row.CreatedAt,
		Items:           items,
	}
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func ItemFromRow(row dbgen.ContextItem) (Item, error) {
	metadata, err := decodeJSONField("context item metadata", row.MetadataJson)
	if err != nil {
		return Item{}, err
	}
	item := Item{
		ID:                row.ID,
		ContextBundleID:   row.ContextBundleID,
		Kind:              ItemKind(row.Kind),
		Path:              nullableString(row.Path),
		StartLine:         nullableInt64(row.StartLine),
		EndLine:           nullableInt64(row.EndLine),
		Title:             nullableString(row.Title),
		ContentArtifactID: nullableString(row.ContentArtifactID),
		TokenEstimate:     row.TokenEstimate,
		Metadata:          metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func decodeJSONField(name string, raw string) (json.RawMessage, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	data := json.RawMessage(raw)
	if !validRawJSON(data) {
		return nil, fmt.Errorf("%s must be valid JSON", name)
	}
	return append(json.RawMessage(nil), data...), nil
}

func validRawJSON(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	return json.Valid(raw)
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}
