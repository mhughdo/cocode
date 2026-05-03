package contextbundle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

var (
	ErrReviewSessionNotFound      = errors.New("review session was not found")
	ErrAgentConfigNotFound        = errors.New("agent config was not found")
	ErrInvalidReviewContextPolicy = errors.New("review context policy is invalid")
	ErrInvalidReviewContextSource = errors.New("review context source is invalid")
)

type Service struct {
	Queries   *dbgen.Queries
	Artifacts *artifact.Store
	Searcher  CodeSearcher
	Now       func() time.Time
}

type BuildReviewContextParams struct {
	ReviewSessionID string
	AgentConfigID   string
	PolicyOverride  json.RawMessage
	Persist         bool
}

type BuildReviewContextResult struct {
	Bundle                  Bundle
	Dropped                 []DroppedItem
	Warnings                []string
	RedactionReport         RedactionReport
	Artifact                dbgen.Artifact
	RedactionReportArtifact dbgen.Artifact
	Persisted               bool
	ResolvedPolicy          ReviewContextPolicy
}

func (s Service) BuildReviewContext(ctx context.Context, params BuildReviewContextParams) (BuildReviewContextResult, error) {
	if s.Queries == nil {
		return BuildReviewContextResult{}, errors.New("context bundle queries are required")
	}
	if s.Artifacts == nil {
		return BuildReviewContextResult{}, errors.New("artifact store is required")
	}
	sessionID := strings.TrimSpace(params.ReviewSessionID)
	if sessionID == "" {
		return BuildReviewContextResult{}, fmt.Errorf("%w: review session id is required", ErrInvalidReviewContextSource)
	}

	session, err := s.Queries.GetReviewSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BuildReviewContextResult{}, ErrReviewSessionNotFound
		}
		return BuildReviewContextResult{}, fmt.Errorf("read review session: %w", err)
	}
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BuildReviewContextResult{}, fmt.Errorf("%w: repository was not found", ErrInvalidReviewContextSource)
		}
		return BuildReviewContextResult{}, fmt.Errorf("read repository: %w", err)
	}
	if repository.WorkspaceID != session.WorkspaceID {
		return BuildReviewContextResult{}, fmt.Errorf("%w: repository does not belong to review session workspace", ErrInvalidReviewContextSource)
	}
	snapshot, err := s.Queries.GetPullRequestSnapshot(ctx, session.SnapshotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BuildReviewContextResult{}, fmt.Errorf("%w: snapshot was not found", ErrInvalidReviewContextSource)
		}
		return BuildReviewContextResult{}, fmt.Errorf("read snapshot: %w", err)
	}
	if snapshot.RepositoryID != repository.ID {
		return BuildReviewContextResult{}, fmt.Errorf("%w: snapshot does not belong to review session repository", ErrInvalidReviewContextSource)
	}
	agentConfigID := strings.TrimSpace(params.AgentConfigID)
	if agentConfigID != "" {
		if _, err := s.Queries.GetAgentConfig(ctx, agentConfigID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return BuildReviewContextResult{}, ErrAgentConfigNotFound
			}
			return BuildReviewContextResult{}, fmt.Errorf("read agent config: %w", err)
		}
	}

	policy, err := DecodeReviewContextPolicy(json.RawMessage(session.ContextPolicyJson))
	if err != nil {
		return BuildReviewContextResult{}, fmt.Errorf("%w: %v", ErrInvalidReviewContextPolicy, err)
	}
	policy, err = ApplyReviewContextPolicy(policy, params.PolicyOverride)
	if err != nil {
		return BuildReviewContextResult{}, fmt.Errorf("%w: %v", ErrInvalidReviewContextPolicy, err)
	}
	depth := ReviewDepth(strings.TrimSpace(session.ReviewDepth))
	if depth == "" {
		depth = ReviewDepthStandard
	}
	if !depth.Valid() {
		return BuildReviewContextResult{}, fmt.Errorf("%w: review depth %q is invalid", ErrInvalidReviewContextSource, session.ReviewDepth)
	}

	createdAt := s.now().UTC().Format(time.RFC3339Nano)
	bundleID := reviewContextBundleID(session.ID, agentConfigID, createdAt)
	result := BuildReviewContextResult{ResolvedPolicy: policy}
	files, err := s.Queries.ListChangedFilesBySnapshot(ctx, snapshot.ID)
	if err != nil {
		return BuildReviewContextResult{}, fmt.Errorf("list changed files: %w", err)
	}

	items := []Item{}
	if policy.IncludePromptMaterial {
		item, err := reviewPromptMaterialItem(bundleID, session, snapshot, len(files), policy)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		items = append(items, item)
	}
	if policy.IncludeChangedCode {
		diffFiles, warnings := s.diffContextFiles(ctx, files)
		result.Warnings = append(result.Warnings, warnings...)
		diffItems, err := BuildDiffContextItems(bundleID, diffFiles)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		items = append(items, diffItems...)

		contentInputs, err := changedFileContentInputs(files)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		contentItems, err := BuildChangedFileContentItems(FileContextOptions{
			BundleID: bundleID,
			RepoRoot: repository.LocalPath,
		}, contentInputs)
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "changed file content context skipped: "+err.Error())
		} else {
			items = append(items, contentItems...)
		}
	}
	if policy.IncludeRelatedCallSites {
		relatedItems, err := BuildRelatedCodeContextItems(ctx, RelatedCodeSearchOptions{
			BundleID: bundleID,
			RepoRoot: repository.LocalPath,
			Searcher: s.Searcher,
		}, relatedSearchInputs(files))
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "related code context skipped: "+err.Error())
		} else {
			items = append(items, relatedItems...)
		}
	}
	if policy.IncludeRelatedTests {
		testItems, err := BuildRelatedTestContextItems(RelatedTestOptions{
			BundleID: bundleID,
			RepoRoot: repository.LocalPath,
		}, relatedTestInputs(files))
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "related test context skipped: "+err.Error())
		} else {
			items = append(items, testItems...)
		}
	}
	if policy.IncludeProjectConventions {
		ruleItems, err := BuildProjectRuleContextItems(ProjectRuleOptions{
			BundleID: bundleID,
			RepoRoot: repository.LocalPath,
		})
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "project convention context skipped: "+err.Error())
		} else {
			items = append(items, ruleItems...)
		}
	}
	if policy.IncludePriorComments {
		commentItems, warnings, err := s.priorCommentItems(ctx, bundleID, snapshot)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		result.Warnings = append(result.Warnings, warnings...)
		items = append(items, commentItems...)
	}
	if policy.IncludePriorDecisions {
		rules, err := s.Queries.ListEnabledReviewRulesByWorkspace(ctx, session.WorkspaceID)
		if err != nil {
			return BuildReviewContextResult{}, fmt.Errorf("list review rules: %w", err)
		}
		decisionItems, err := BuildPriorDecisionContextItems(PriorDecisionOptions{
			BundleID:    bundleID,
			WorkspaceID: session.WorkspaceID,
		}, rules)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		items = append(items, decisionItems...)
	}

	bundle := Bundle{
		ID:              bundleID,
		ReviewSessionID: session.ID,
		AgentConfigID:   agentConfigID,
		Scope:           ScopeReview,
		Policy:          policy.JSON(),
		CreatedAt:       createdAt,
		Items:           items,
	}
	bundle = ApplyBundleTokenEstimates(bundle)
	bundle, result.Dropped, err = BudgetBundle(bundle, BudgetOptions{
		Depth:     depth,
		MaxTokens: policy.MaxTokens,
		MaxItems:  policy.MaxItems,
	})
	if err != nil {
		return BuildReviewContextResult{}, err
	}
	if policy.RedactSecrets {
		bundle, result.RedactionReport, err = RedactBundle(bundle, RedactionOptions{})
		if err != nil {
			return BuildReviewContextResult{}, err
		}
	}

	if params.Persist && result.RedactionReport.RedactionCount > 0 {
		artifactID := reviewContextArtifactID("redaction", bundle.ID)
		result.RedactionReportArtifact, err = SaveRedactionReportArtifact(ctx, s.Artifacts, RedactionArtifactParams{
			ID:              artifactID,
			WorkspaceID:     session.WorkspaceID,
			ReviewSessionID: session.ID,
			BundleID:        bundle.ID,
			CreatedAt:       createdAt,
		}, result.RedactionReport)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
	}
	if params.Persist {
		persisted, err := (Persister{Queries: s.Queries, Artifacts: s.Artifacts}).PersistRenderedBundle(ctx, PersistParams{
			WorkspaceID: session.WorkspaceID,
			Bundle:      bundle,
			ArtifactID:  reviewContextArtifactID("bundle", bundle.ID),
			CreatedAt:   createdAt,
		})
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		bundle.ArtifactID = persisted.Bundle.ArtifactID
		bundle.TokenEstimate = persisted.Bundle.TokenEstimate
		bundle.ItemCount = persisted.Bundle.ItemCount
		result.Artifact = persisted.Artifact
		result.Persisted = true
	}

	result.Bundle = bundle
	return result, nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Service) diffContextFiles(ctx context.Context, files []dbgen.ChangedFile) ([]DiffContextFile, []string) {
	out := []DiffContextFile{}
	warnings := []string{}
	for _, file := range files {
		if !file.PatchArtifactID.Valid || strings.TrimSpace(file.PatchArtifactID.String) == "" {
			continue
		}
		content, _, err := s.Artifacts.Read(ctx, file.PatchArtifactID.String)
		if err != nil {
			warnings = appendWarning(warnings, fmt.Sprintf("patch artifact for %s could not be read: %v", file.Path, err))
			continue
		}
		parsed, err := diffparse.Parse(syntheticPatchForChangedFile(file, string(content)))
		if err != nil {
			warnings = appendWarning(warnings, fmt.Sprintf("patch artifact for %s could not be parsed: %v", file.Path, err))
			continue
		}
		parsedFile, ok := selectParsedFile(file, parsed)
		if !ok || len(parsedFile.Hunks) == 0 {
			continue
		}
		out = append(out, DiffContextFile{
			ChangedFileID:   file.ID,
			PatchArtifactID: nullableDBString(file.PatchArtifactID),
			Path:            file.Path,
			OldPath:         nullableDBString(file.OldPath),
			Status:          file.Status,
			Binary:          file.IsBinary != 0,
			Excluded:        file.IsExcluded != 0,
			Hunks:           append([]diffparse.Hunk(nil), parsedFile.Hunks...),
		})
	}
	return out, warnings
}

func (s Service) priorCommentItems(ctx context.Context, bundleID string, snapshot dbgen.PullRequestSnapshot) ([]Item, []string, error) {
	artifactID := previousCommentsArtifactID(snapshot.MetadataJson)
	if artifactID == "" {
		return nil, nil, nil
	}
	content, _, err := s.Artifacts.Read(ctx, artifactID)
	if err != nil {
		return nil, []string{"prior comments context skipped: " + err.Error()}, nil
	}
	items, err := BuildPriorCommentContextItemsFromJSON(PriorCommentOptions{
		BundleID:                   bundleID,
		PreviousCommentsArtifactID: artifactID,
	}, content)
	if err != nil {
		return nil, nil, err
	}
	return items, nil, nil
}

func reviewPromptMaterialItem(bundleID string, session dbgen.ReviewSession, snapshot dbgen.PullRequestSnapshot, changedFileCount int, policy ReviewContextPolicy) (Item, error) {
	var builder strings.Builder
	builder.WriteString("Review session: ")
	builder.WriteString(session.Title)
	builder.WriteByte('\n')
	builder.WriteString("Review depth: ")
	builder.WriteString(session.ReviewDepth)
	builder.WriteByte('\n')
	if strings.TrimSpace(session.FocusPrompt.String) != "" {
		builder.WriteString("Focus: ")
		builder.WriteString(strings.TrimSpace(session.FocusPrompt.String))
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(session.Preset.String) != "" {
		builder.WriteString("Preset: ")
		builder.WriteString(strings.TrimSpace(session.Preset.String))
		builder.WriteByte('\n')
	}
	builder.WriteString("Snapshot: ")
	builder.WriteString(snapshot.SourceType)
	if snapshot.PrNumber.Valid {
		builder.WriteString(fmt.Sprintf(" PR #%d", snapshot.PrNumber.Int64))
	}
	if snapshot.PrTitle.Valid && strings.TrimSpace(snapshot.PrTitle.String) != "" {
		builder.WriteString(" - ")
		builder.WriteString(strings.TrimSpace(snapshot.PrTitle.String))
	}
	builder.WriteByte('\n')
	if snapshot.PrUrl.Valid && strings.TrimSpace(snapshot.PrUrl.String) != "" {
		builder.WriteString("URL: ")
		builder.WriteString(strings.TrimSpace(snapshot.PrUrl.String))
		builder.WriteByte('\n')
	}
	if snapshot.BaseRef.Valid || snapshot.HeadRef.Valid {
		builder.WriteString("Refs: ")
		builder.WriteString(strings.TrimSpace(snapshot.BaseRef.String))
		builder.WriteString(" -> ")
		builder.WriteString(strings.TrimSpace(snapshot.HeadRef.String))
		builder.WriteByte('\n')
	}
	builder.WriteString(fmt.Sprintf("Changed files: %d\n", changedFileCount))
	if len(policy.LocalOnlyPaths) > 0 {
		builder.WriteString("Local-only paths:\n")
		for _, path := range policy.LocalOnlyPaths {
			builder.WriteString("- ")
			builder.WriteString(path)
			builder.WriteByte('\n')
		}
	}

	metadata, err := json.Marshal(map[string]any{
		"source":             "review_session",
		"workspace_id":       session.WorkspaceID,
		"repository_id":      session.RepositoryID,
		"snapshot_id":        session.SnapshotID,
		"snapshot_source":    snapshot.SourceType,
		"changed_file_count": changedFileCount,
	})
	if err != nil {
		return Item{}, fmt.Errorf("encode prompt material metadata: %w", err)
	}
	content := builder.String()
	item := Item{
		ID:              stableContextItemID(bundleID, "prompt_material", 0),
		ContextBundleID: bundleID,
		Kind:            ItemPromptMaterial,
		Title:           "Review prompt material",
		Content:         content,
		TokenEstimate:   estimateTokens(content),
		Metadata:        metadata,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func changedFileContentInputs(files []dbgen.ChangedFile) ([]ChangedFileContentInput, error) {
	inputs := make([]ChangedFileContentInput, 0, len(files))
	for _, file := range files {
		input, err := ChangedFileContentInputFromRow(file)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func relatedSearchInputs(files []dbgen.ChangedFile) []RelatedSearchInput {
	inputs := make([]RelatedSearchInput, 0, len(files))
	for _, file := range files {
		inputs = append(inputs, RelatedSearchInput{
			ChangedFileID: file.ID,
			Path:          file.Path,
			Excluded:      file.IsExcluded != 0,
			Binary:        file.IsBinary != 0,
		})
	}
	return inputs
}

func relatedTestInputs(files []dbgen.ChangedFile) []RelatedTestInput {
	inputs := make([]RelatedTestInput, 0, len(files))
	for _, file := range files {
		inputs = append(inputs, RelatedTestInput{
			ChangedFileID: file.ID,
			Path:          file.Path,
			Excluded:      file.IsExcluded != 0,
			Binary:        file.IsBinary != 0,
		})
	}
	return inputs
}

func syntheticPatchForChangedFile(file dbgen.ChangedFile, content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "diff --git ") {
		return content
	}
	oldPath := nullableDBString(file.OldPath)
	if oldPath == "" {
		oldPath = file.Path
	}
	oldHeader := "a/" + oldPath
	newHeader := "b/" + file.Path
	switch strings.ToLower(strings.TrimSpace(file.Status)) {
	case string(diffparse.StatusAdded):
		oldHeader = "/dev/null"
	case string(diffparse.StatusDeleted):
		newHeader = "/dev/null"
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return fmt.Sprintf("diff --git a/%s b/%s\n--- %s\n+++ %s\n%s", oldPath, file.Path, oldHeader, newHeader, content)
}

func selectParsedFile(file dbgen.ChangedFile, parsed []diffparse.File) (diffparse.File, bool) {
	if len(parsed) == 0 {
		return diffparse.File{}, false
	}
	for _, candidate := range parsed {
		if candidate.Path == file.Path || candidate.OldPath == file.Path || candidate.OldPath == nullableDBString(file.OldPath) {
			return candidate, true
		}
	}
	return parsed[0], true
}

func previousCommentsArtifactID(metadataJSON string) string {
	var metadata struct {
		PreviousComments struct {
			ArtifactID string `json:"artifact_id"`
		} `json:"previous_comments"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return ""
	}
	return strings.TrimSpace(metadata.PreviousComments.ArtifactID)
}

func reviewContextBundleID(sessionID string, agentConfigID string, createdAt string) string {
	key := strings.TrimSpace(sessionID) + "\x00" + strings.TrimSpace(agentConfigID) + "\x00" + createdAt
	sum := sha256.Sum256([]byte(key))
	return "bundle_" + hex.EncodeToString(sum[:12])
}

func reviewContextArtifactID(kind string, bundleID string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + bundleID))
	return "artifact_context_" + kind + "_" + hex.EncodeToString(sum[:12])
}

func appendWarning(warnings []string, warning string) []string {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return warnings
	}
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	if len(warnings) >= 20 {
		return warnings
	}
	return append(warnings, warning)
}

func nullableDBString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
