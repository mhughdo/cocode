package contextbundle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	defaultFindingContextMaxTokens      int64 = 8_000
	defaultEvidenceMapContextMaxTokens  int64 = 10_000
	defaultFindingContextMaxItems             = 80
	defaultEvidenceMapContextMaxItems         = 120
	defaultFindingEvidenceItemLimit           = 40
	defaultFindingRelatedItemLimit            = 24
	defaultFindingRelatedMatchesPerTerm       = 8
	defaultFindingRelatedTestLimit            = 24
	defaultFindingContextSnippetBytes         = 12 * 1024
)

type BuildFindingContextParams struct {
	ReviewSessionID string
	FindingID       string
	PolicyOverride  json.RawMessage
	Persist         bool
}

type BuildEvidenceMapContextParams struct {
	ReviewSessionID string
	FindingID       string
	PolicyOverride  json.RawMessage
	Persist         bool
}

type scopedContextSource struct {
	session    dbgen.ReviewSession
	repository dbgen.Repository
	snapshot   dbgen.PullRequestSnapshot
	finding    dbgen.Finding
	files      []dbgen.ChangedFile
	evidence   []dbgen.EvidenceItem
}

func (s Service) BuildFindingContext(ctx context.Context, params BuildFindingContextParams) (BuildReviewContextResult, error) {
	source, policy, depth, err := s.scopedContextSource(ctx, params.ReviewSessionID, params.FindingID, params.PolicyOverride, ScopeFinding)
	if err != nil {
		return BuildReviewContextResult{}, err
	}
	return s.buildScopedContext(ctx, scopedContextBuildParams{
		source:  source,
		policy:  policy,
		depth:   depth,
		scope:   ScopeFinding,
		persist: params.Persist,
	})
}

func (s Service) BuildEvidenceMapContext(ctx context.Context, params BuildEvidenceMapContextParams) (BuildReviewContextResult, error) {
	source, policy, depth, err := s.scopedContextSource(ctx, params.ReviewSessionID, params.FindingID, params.PolicyOverride, ScopeEvidenceMap)
	if err != nil {
		return BuildReviewContextResult{}, err
	}
	return s.buildScopedContext(ctx, scopedContextBuildParams{
		source:             source,
		policy:             policy,
		depth:              depth,
		scope:              ScopeEvidenceMap,
		persist:            params.Persist,
		includeEvidenceMap: true,
	})
}

type scopedContextBuildParams struct {
	source             scopedContextSource
	policy             ReviewContextPolicy
	depth              ReviewDepth
	scope              Scope
	persist            bool
	includeEvidenceMap bool
}

func (s Service) scopedContextSource(ctx context.Context, reviewSessionID string, findingID string, override json.RawMessage, scope Scope) (scopedContextSource, ReviewContextPolicy, ReviewDepth, error) {
	if s.Queries == nil {
		return scopedContextSource{}, ReviewContextPolicy{}, "", errors.New("context bundle queries are required")
	}
	if s.Artifacts == nil {
		return scopedContextSource{}, ReviewContextPolicy{}, "", errors.New("artifact store is required")
	}
	findingID = strings.TrimSpace(findingID)
	if findingID == "" {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: finding id is required", ErrInvalidReviewContextSource)
	}
	finding, err := s.Queries.GetFinding(ctx, findingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: finding was not found", ErrInvalidReviewContextSource)
		}
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("read finding: %w", err)
	}
	sessionID := strings.TrimSpace(reviewSessionID)
	if sessionID == "" {
		sessionID = finding.ReviewSessionID
	}
	if sessionID == "" || sessionID != finding.ReviewSessionID {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: finding does not belong to review session", ErrInvalidReviewContextSource)
	}
	session, err := s.Queries.GetReviewSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scopedContextSource{}, ReviewContextPolicy{}, "", ErrReviewSessionNotFound
		}
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("read review session: %w", err)
	}
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: repository was not found", ErrInvalidReviewContextSource)
		}
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("read repository: %w", err)
	}
	if repository.WorkspaceID != session.WorkspaceID {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: repository does not belong to review session workspace", ErrInvalidReviewContextSource)
	}
	snapshot, err := s.Queries.GetPullRequestSnapshot(ctx, session.SnapshotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: snapshot was not found", ErrInvalidReviewContextSource)
		}
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("read snapshot: %w", err)
	}
	if snapshot.RepositoryID != repository.ID {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: snapshot does not belong to review session repository", ErrInvalidReviewContextSource)
	}
	policy, err := DecodeReviewContextPolicy(json.RawMessage(session.ContextPolicyJson))
	if err != nil {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: %v", ErrInvalidReviewContextPolicy, err)
	}
	policy, err = ApplyReviewContextPolicy(policy, override)
	if err != nil {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: %v", ErrInvalidReviewContextPolicy, err)
	}
	policy = scopedPolicy(policy, scope)
	depth := ReviewDepth(strings.TrimSpace(session.ReviewDepth))
	if depth == "" {
		depth = ReviewDepthStandard
	}
	if !depth.Valid() {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("%w: review depth %q is invalid", ErrInvalidReviewContextSource, session.ReviewDepth)
	}
	files, err := s.Queries.ListChangedFilesBySnapshot(ctx, snapshot.ID)
	if err != nil {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("list changed files: %w", err)
	}
	evidence, err := s.Queries.ListEvidenceItemsByFinding(ctx, finding.ID)
	if err != nil {
		return scopedContextSource{}, ReviewContextPolicy{}, "", fmt.Errorf("list finding evidence: %w", err)
	}
	return scopedContextSource{
		session:    session,
		repository: repository,
		snapshot:   snapshot,
		finding:    finding,
		files:      files,
		evidence:   evidence,
	}, policy, depth, nil
}

func (s Service) buildScopedContext(ctx context.Context, params scopedContextBuildParams) (BuildReviewContextResult, error) {
	source := params.source
	createdAt := s.now().UTC().Format(time.RFC3339Nano)
	bundleID := scopedContextBundleID(source.session.ID, source.finding.ID, params.scope, createdAt)
	result := BuildReviewContextResult{ResolvedPolicy: params.policy}
	items := []Item{}

	item, err := findingPromptMaterialItem(bundleID, source.session, source.snapshot, source.finding, params.scope)
	if err != nil {
		return BuildReviewContextResult{}, err
	}
	items = append(items, item)

	evidenceItems, warnings, err := findingEvidenceContextItems(bundleID, source.finding, source.evidence)
	if err != nil {
		return BuildReviewContextResult{}, err
	}
	result.Warnings = append(result.Warnings, warnings...)
	items = append(items, evidenceItems...)

	if params.includeEvidenceMap {
		graphItem, graphWarnings, err := s.evidenceMapContextItem(ctx, bundleID, source.finding)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		result.Warnings = append(result.Warnings, graphWarnings...)
		if graphItem.ID != "" {
			items = append(items, graphItem)
		}
	}

	scopedFiles := findingScopedChangedFiles(source.finding, source.evidence, source.files)
	if params.policy.IncludeChangedCode {
		diffFiles, warnings := s.diffContextFiles(ctx, scopedFiles)
		result.Warnings = append(result.Warnings, warnings...)
		diffItems, err := BuildDiffContextItems(bundleID, diffFiles)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		items = append(items, diffItems...)

		inputs, err := changedFileContentInputs(scopedFiles)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		contentItems, err := BuildChangedFileContentItems(FileContextOptions{
			BundleID:         bundleID,
			RepoRoot:         source.repository.LocalPath,
			ContextLines:     6,
			MaxFullFileBytes: 12 * 1024,
			MaxSliceBytes:    6 * 1024,
			MaxTotalBytes:    32 * 1024,
			MaxItems:         24,
		}, inputs)
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "finding changed file content context skipped: "+err.Error())
		} else {
			items = append(items, contentItems...)
		}
	}
	symbols := findingContextSymbols(source.finding, source.evidence)
	if params.policy.IncludeRelatedCallSites && len(scopedFiles) > 0 {
		relatedInputs := relatedSearchInputs(scopedFiles)
		for index := range relatedInputs {
			relatedInputs[index].Symbols = symbols
		}
		relatedItems, err := BuildRelatedCodeContextItems(ctx, RelatedCodeSearchOptions{
			BundleID:          bundleID,
			RepoRoot:          source.repository.LocalPath,
			Searcher:          s.Searcher,
			MaxItems:          defaultFindingRelatedItemLimit,
			MaxMatchesPerTerm: defaultFindingRelatedMatchesPerTerm,
		}, relatedInputs)
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "finding related code context skipped: "+err.Error())
		} else {
			items = append(items, relatedItems...)
		}
	}
	if params.policy.IncludeRelatedTests && len(scopedFiles) > 0 {
		testInputs := relatedTestInputs(scopedFiles)
		for index := range testInputs {
			testInputs[index].Symbols = symbols
		}
		testItems, err := BuildRelatedTestContextItems(RelatedTestOptions{
			BundleID: bundleID,
			RepoRoot: source.repository.LocalPath,
			MaxItems: defaultFindingRelatedTestLimit,
		}, testInputs)
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "finding related test context skipped: "+err.Error())
		} else {
			items = append(items, testItems...)
		}
	}
	if params.policy.IncludeProjectConventions {
		ruleItems, err := BuildProjectRuleContextItems(ProjectRuleOptions{
			BundleID: bundleID,
			RepoRoot: source.repository.LocalPath,
			MaxItems: 8,
		})
		if err != nil {
			result.Warnings = appendWarning(result.Warnings, "finding project convention context skipped: "+err.Error())
		} else {
			items = append(items, ruleItems...)
		}
	}
	if params.policy.IncludePriorDecisions {
		rules, err := s.Queries.ListEnabledReviewRulesByWorkspace(ctx, source.session.WorkspaceID)
		if err != nil {
			return BuildReviewContextResult{}, fmt.Errorf("list review rules: %w", err)
		}
		decisionItems, err := BuildPriorDecisionContextItems(PriorDecisionOptions{
			BundleID:    bundleID,
			WorkspaceID: source.session.WorkspaceID,
			MaxItems:    8,
		}, rules)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
		items = append(items, decisionItems...)
	}

	bundle := Bundle{
		ID:              bundleID,
		ReviewSessionID: source.session.ID,
		Scope:           params.scope,
		Policy:          params.policy.JSON(),
		CreatedAt:       createdAt,
		Items:           items,
	}
	bundle = ApplyBundleTokenEstimates(bundle)
	bundle, result.Dropped, err = BudgetBundle(bundle, BudgetOptions{
		Depth:     params.depth,
		MaxTokens: params.policy.MaxTokens,
		MaxItems:  params.policy.MaxItems,
	})
	if err != nil {
		return BuildReviewContextResult{}, err
	}
	if params.policy.RedactSecrets {
		bundle, result.RedactionReport, err = RedactBundle(bundle, RedactionOptions{})
		if err != nil {
			return BuildReviewContextResult{}, err
		}
	}
	if params.persist && result.RedactionReport.RedactionCount > 0 {
		artifactID := reviewContextArtifactID("redaction", bundle.ID)
		result.RedactionReportArtifact, err = SaveRedactionReportArtifact(ctx, s.Artifacts, RedactionArtifactParams{
			ID:              artifactID,
			WorkspaceID:     source.session.WorkspaceID,
			ReviewSessionID: source.session.ID,
			BundleID:        bundle.ID,
			CreatedAt:       createdAt,
		}, result.RedactionReport)
		if err != nil {
			return BuildReviewContextResult{}, err
		}
	}
	if params.persist {
		persisted, err := (Persister{Queries: s.Queries, Artifacts: s.Artifacts}).PersistRenderedBundle(ctx, PersistParams{
			WorkspaceID: source.session.WorkspaceID,
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

func findingPromptMaterialItem(bundleID string, session dbgen.ReviewSession, snapshot dbgen.PullRequestSnapshot, finding dbgen.Finding, scope Scope) (Item, error) {
	var builder strings.Builder
	builder.WriteString("Review session: ")
	builder.WriteString(session.Title)
	builder.WriteByte('\n')
	builder.WriteString("Context scope: ")
	builder.WriteString(string(scope))
	builder.WriteByte('\n')
	builder.WriteString("Finding ID: ")
	builder.WriteString(finding.ID)
	builder.WriteByte('\n')
	builder.WriteString("Severity: ")
	builder.WriteString(finding.Severity)
	builder.WriteByte('\n')
	builder.WriteString("Verification status: ")
	builder.WriteString(finding.VerificationStatus)
	builder.WriteByte('\n')
	if finding.PrimaryPath.Valid {
		builder.WriteString("Primary location: ")
		builder.WriteString(finding.PrimaryPath.String)
		if finding.PrimaryStartLine.Valid {
			builder.WriteString(fmt.Sprintf(":%d", finding.PrimaryStartLine.Int64))
			if finding.PrimaryEndLine.Valid && finding.PrimaryEndLine.Int64 > finding.PrimaryStartLine.Int64 {
				builder.WriteString(fmt.Sprintf("-%d", finding.PrimaryEndLine.Int64))
			}
		}
		builder.WriteByte('\n')
	}
	if snapshot.PrUrl.Valid && strings.TrimSpace(snapshot.PrUrl.String) != "" {
		builder.WriteString("PR URL: ")
		builder.WriteString(strings.TrimSpace(snapshot.PrUrl.String))
		builder.WriteByte('\n')
	}
	builder.WriteString("\nClaim:\n")
	builder.WriteString(strings.TrimSpace(finding.CanonicalClaim))
	builder.WriteString("\n")
	if finding.EvidenceSummary.Valid && strings.TrimSpace(finding.EvidenceSummary.String) != "" {
		builder.WriteString("\nEvidence summary:\n")
		builder.WriteString(strings.TrimSpace(finding.EvidenceSummary.String))
		builder.WriteString("\n")
	}
	if finding.CounterEvidenceSummary.Valid && strings.TrimSpace(finding.CounterEvidenceSummary.String) != "" {
		builder.WriteString("\nCounter-evidence summary:\n")
		builder.WriteString(strings.TrimSpace(finding.CounterEvidenceSummary.String))
		builder.WriteString("\n")
	}
	if finding.SuggestedFix.Valid && strings.TrimSpace(finding.SuggestedFix.String) != "" {
		builder.WriteString("\nSuggested fix:\n")
		builder.WriteString(strings.TrimSpace(finding.SuggestedFix.String))
		builder.WriteString("\n")
	}
	metadata, err := json.Marshal(map[string]any{
		"source":              "finding_prompt_material",
		"workspace_id":        session.WorkspaceID,
		"repository_id":       session.RepositoryID,
		"snapshot_id":         session.SnapshotID,
		"snapshot_source":     snapshot.SourceType,
		"finding_id":          finding.ID,
		"verification_status": finding.VerificationStatus,
	})
	if err != nil {
		return Item{}, fmt.Errorf("encode finding prompt metadata: %w", err)
	}
	item := Item{
		ID:              stableContextItemID(bundleID, "finding_prompt_material", 0),
		ContextBundleID: bundleID,
		Kind:            ItemPromptMaterial,
		Path:            nullableString(finding.PrimaryPath),
		StartLine:       lineStartValue(finding.PrimaryStartLine),
		EndLine:         lineEndValue(finding.PrimaryStartLine, finding.PrimaryEndLine),
		Title:           "Finding prompt material",
		Content:         builder.String(),
		Metadata:        metadata,
	}
	item = ApplyItemTokenEstimate(item)
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func findingEvidenceContextItems(bundleID string, finding dbgen.Finding, rows []dbgen.EvidenceItem) ([]Item, []string, error) {
	rows = append([]dbgen.EvidenceItem(nil), rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		leftRank := evidenceContextKindRank(rows[i].Kind)
		rightRank := evidenceContextKindRank(rows[j].Kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if rows[i].Confidence != rows[j].Confidence {
			return rows[i].Confidence > rows[j].Confidence
		}
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt < rows[j].CreatedAt
		}
		return rows[i].ID < rows[j].ID
	})
	warnings := []string{}
	if len(rows) > defaultFindingEvidenceItemLimit {
		warnings = append(warnings, fmt.Sprintf("%d finding evidence item(s) omitted from scoped context.", len(rows)-defaultFindingEvidenceItemLimit))
		rows = rows[:defaultFindingEvidenceItemLimit]
	}
	items := make([]Item, 0, len(rows))
	for index, row := range rows {
		content, truncated := renderEvidenceContextContent(row)
		metadata, err := json.Marshal(map[string]any{
			"source":            "finding_evidence",
			"finding_id":        finding.ID,
			"evidence_item_id":  row.ID,
			"evidence_kind":     row.Kind,
			"confidence":        row.Confidence,
			"snippet_truncated": truncated,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("encode finding evidence metadata: %w", err)
		}
		item := Item{
			ID:              stableContextItemID(bundleID, "evidence:"+row.ID, index),
			ContextBundleID: bundleID,
			Kind:            ItemEvidence,
			Path:            nullableString(row.Path),
			StartLine:       lineStartValue(row.StartLine),
			EndLine:         lineEndValue(row.StartLine, row.EndLine),
			Title:           "Evidence: " + row.Title,
			Content:         content,
			Metadata:        metadata,
		}
		item = ApplyItemTokenEstimate(item)
		if err := item.Validate(); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	return items, warnings, nil
}

func (s Service) evidenceMapContextItem(ctx context.Context, bundleID string, finding dbgen.Finding) (Item, []string, error) {
	graph, err := s.Queries.GetEvidenceGraphByFinding(ctx, finding.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, []string{"evidence map context skipped: graph has not been built"}, nil
		}
		return Item{}, nil, fmt.Errorf("read evidence map graph: %w", err)
	}
	nodes, err := s.Queries.ListEvidenceNodesByGraph(ctx, graph.ID)
	if err != nil {
		return Item{}, nil, fmt.Errorf("list evidence map nodes: %w", err)
	}
	edges, err := s.Queries.ListEvidenceEdgesByGraph(ctx, graph.ID)
	if err != nil {
		return Item{}, nil, fmt.Errorf("list evidence map edges: %w", err)
	}
	paths, err := s.Queries.ListCallPathsByGraph(ctx, graph.ID)
	if err != nil {
		return Item{}, nil, fmt.Errorf("list evidence map call paths: %w", err)
	}
	var builder strings.Builder
	builder.WriteString("Evidence graph: ")
	builder.WriteString(graph.ID)
	builder.WriteString("\nStatus: ")
	builder.WriteString(graph.Status)
	if graph.Summary.Valid && strings.TrimSpace(graph.Summary.String) != "" {
		builder.WriteString("\nSummary: ")
		builder.WriteString(strings.TrimSpace(graph.Summary.String))
	}
	builder.WriteString("\n\nNodes:\n")
	for _, node := range nodes {
		builder.WriteString("- ")
		builder.WriteString(node.Kind)
		builder.WriteString(": ")
		builder.WriteString(node.Label)
		if node.Path.Valid {
			builder.WriteString(" (")
			builder.WriteString(node.Path.String)
			if node.StartLine.Valid {
				builder.WriteString(fmt.Sprintf(":%d", node.StartLine.Int64))
			}
			builder.WriteString(")")
		}
		builder.WriteByte('\n')
	}
	builder.WriteString("\nEdges:\n")
	for _, edge := range edges {
		builder.WriteString("- ")
		builder.WriteString(edge.Kind)
		builder.WriteString(" ")
		builder.WriteString(edge.Status)
		if edge.Label.Valid && strings.TrimSpace(edge.Label.String) != "" {
			builder.WriteString(": ")
			builder.WriteString(strings.TrimSpace(edge.Label.String))
		}
		builder.WriteString(" [")
		builder.WriteString(edge.SourceNodeID)
		builder.WriteString(" -> ")
		builder.WriteString(edge.TargetNodeID)
		builder.WriteString("]\n")
	}
	for _, path := range paths {
		steps, err := s.Queries.ListCallPathStepsByCallPath(ctx, path.ID)
		if err != nil {
			return Item{}, nil, fmt.Errorf("list evidence map call path steps: %w", err)
		}
		builder.WriteString("\nCall path")
		if path.Label.Valid && strings.TrimSpace(path.Label.String) != "" {
			builder.WriteString(": ")
			builder.WriteString(strings.TrimSpace(path.Label.String))
		}
		builder.WriteByte('\n')
		for _, step := range steps {
			builder.WriteString(fmt.Sprintf("%d. %s", step.StepIndex+1, step.Label))
			if step.Path.Valid {
				builder.WriteString(" - ")
				builder.WriteString(step.Path.String)
				if step.StartLine.Valid {
					builder.WriteString(fmt.Sprintf(":%d", step.StartLine.Int64))
				}
			}
			builder.WriteByte('\n')
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"source":            "evidence_map_context",
		"finding_id":        finding.ID,
		"evidence_graph_id": graph.ID,
		"status":            graph.Status,
		"node_count":        len(nodes),
		"edge_count":        len(edges),
		"call_path_count":   len(paths),
	})
	if err != nil {
		return Item{}, nil, fmt.Errorf("encode evidence map context metadata: %w", err)
	}
	item := Item{
		ID:              stableContextItemID(bundleID, "evidence_map:"+graph.ID, 0),
		ContextBundleID: bundleID,
		Kind:            ItemEvidence,
		Title:           "Evidence Map graph",
		Content:         builder.String(),
		Metadata:        metadata,
	}
	item = ApplyItemTokenEstimate(item)
	if err := item.Validate(); err != nil {
		return Item{}, nil, err
	}
	return item, nil, nil
}

func renderEvidenceContextContent(row dbgen.EvidenceItem) (string, bool) {
	var builder strings.Builder
	builder.WriteString("Kind: ")
	builder.WriteString(row.Kind)
	builder.WriteByte('\n')
	builder.WriteString("Title: ")
	builder.WriteString(row.Title)
	builder.WriteByte('\n')
	builder.WriteString("Summary: ")
	builder.WriteString(row.Summary)
	builder.WriteByte('\n')
	if row.Path.Valid && strings.TrimSpace(row.Path.String) != "" {
		builder.WriteString("Location: ")
		builder.WriteString(row.Path.String)
		if row.StartLine.Valid {
			builder.WriteString(fmt.Sprintf(":%d", row.StartLine.Int64))
			if row.EndLine.Valid && row.EndLine.Int64 > row.StartLine.Int64 {
				builder.WriteString(fmt.Sprintf("-%d", row.EndLine.Int64))
			}
		}
		builder.WriteByte('\n')
	}
	builder.WriteString(fmt.Sprintf("Confidence: %.2f\n", row.Confidence))
	snippet, truncated := evidenceCodeSnippet(row.MetadataJson, defaultFindingContextSnippetBytes)
	if snippet != "" {
		builder.WriteString("\nSnippet:\n")
		builder.WriteString(snippet)
		if truncated {
			builder.WriteString("\n[truncated]\n")
		}
	}
	return builder.String(), truncated
}

func findingScopedChangedFiles(finding dbgen.Finding, evidence []dbgen.EvidenceItem, files []dbgen.ChangedFile) []dbgen.ChangedFile {
	paths := map[string]struct{}{}
	if finding.PrimaryPath.Valid {
		addScopedPath(paths, finding.PrimaryPath.String)
	}
	for _, item := range evidence {
		if item.Path.Valid {
			addScopedPath(paths, item.Path.String)
		}
	}
	scoped := make([]dbgen.ChangedFile, 0, min(len(paths), len(files)))
	for _, file := range files {
		if _, ok := paths[cleanScopedPath(file.Path)]; ok {
			scoped = append(scoped, file)
		}
	}
	return scoped
}

func findingContextSymbols(finding dbgen.Finding, evidence []dbgen.EvidenceItem) []string {
	terms := []string{}
	for _, value := range []string{
		finding.CanonicalClaim,
		nullableString(finding.SuggestedFix),
		nullableString(finding.DraftComment),
	} {
		for _, token := range scopedTokens(value) {
			addScopedTerm(&terms, token)
			if len(terms) >= 8 {
				return terms
			}
		}
	}
	for _, item := range evidence {
		for _, token := range scopedTokens(item.Title + " " + item.Summary) {
			addScopedTerm(&terms, token)
			if len(terms) >= 8 {
				return terms
			}
		}
	}
	return terms
}

func scopedTokens(value string) []string {
	stop := map[string]struct{}{
		"finding": {}, "evidence": {}, "summary": {}, "without": {}, "with": {}, "from": {}, "this": {}, "that": {}, "when": {}, "where": {}, "does": {}, "should": {}, "would": {}, "could": {}, "missing": {}, "lacks": {},
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len(field) < 4 {
			continue
		}
		lower := strings.ToLower(field)
		if _, ok := stop[lower]; ok {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

func scopedPolicy(policy ReviewContextPolicy, scope Scope) ReviewContextPolicy {
	maxTokens := defaultFindingContextMaxTokens
	maxItems := defaultFindingContextMaxItems
	if scope == ScopeEvidenceMap {
		maxTokens = defaultEvidenceMapContextMaxTokens
		maxItems = defaultEvidenceMapContextMaxItems
	}
	if policy.MaxTokens <= 0 || policy.MaxTokens > maxTokens {
		policy.MaxTokens = maxTokens
	}
	if policy.MaxItems <= 0 || policy.MaxItems > maxItems {
		policy.MaxItems = maxItems
	}
	return policy
}

func evidenceCodeSnippet(raw string, limit int) (string, bool) {
	var payload struct {
		CodeSnippet string `json:"code_snippet"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", false
	}
	snippet := strings.TrimSpace(payload.CodeSnippet)
	if snippet == "" || limit <= 0 || len(snippet) <= limit {
		return snippet, false
	}
	return snippet[:limit], true
}

func evidenceContextKindRank(kind string) int {
	switch kind {
	case "supporting":
		return 0
	case "counter":
		return 1
	case "test":
		return 2
	case "missing":
		return 3
	case "search":
		return 4
	case "agent":
		return 5
	case "static_analysis":
		return 6
	default:
		return 7
	}
}

func addScopedPath(paths map[string]struct{}, path string) {
	if path = cleanScopedPath(path); path != "" {
		paths[path] = struct{}{}
	}
}

func cleanScopedPath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	if path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "../") {
		return ""
	}
	return path
}

func addScopedTerm(terms *[]string, term string) {
	term = strings.TrimSpace(term)
	if len(term) < 3 || slices.Contains(*terms, term) {
		return
	}
	*terms = append(*terms, term)
}

func scopedContextBundleID(sessionID string, findingID string, scope Scope, createdAt string) string {
	key := strings.TrimSpace(sessionID) + "\x00" + strings.TrimSpace(findingID) + "\x00" + string(scope) + "\x00" + createdAt
	sum := sha256.Sum256([]byte(key))
	return "bundle_" + hex.EncodeToString(sum[:12])
}

func lineStartValue(start sql.NullInt64) int64 {
	if !start.Valid || start.Int64 <= 0 {
		return 0
	}
	return start.Int64
}

func lineEndValue(start sql.NullInt64, end sql.NullInt64) int64 {
	if !start.Valid || start.Int64 <= 0 {
		return 0
	}
	if end.Valid && end.Int64 >= start.Int64 {
		return end.Int64
	}
	return start.Int64
}
