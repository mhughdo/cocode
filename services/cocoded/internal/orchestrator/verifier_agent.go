package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
	"github.com/hughdo/cocode/services/cocoded/internal/agentrun"
	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/evidence"
)

const (
	defaultVerifierAgentLimit        = 2
	defaultVerifierFindingLimit      = 20
	defaultVerifierEvidenceItemLimit = 12
	defaultVerifierTextSummaryBytes  = 4 * 1024
)

type verifierAgentSummary struct {
	Configured            int            `json:"configured"`
	FindingsEligible      int            `json:"findings_eligible"`
	FindingsAttempted     int            `json:"findings_attempted"`
	RunsStarted           int            `json:"runs_started"`
	RunsSucceeded         int            `json:"runs_succeeded"`
	RunsFailed            int            `json:"runs_failed"`
	EvidenceItemsCreated  int            `json:"evidence_items_created"`
	StatusUpdates         int            `json:"status_updates"`
	ByVerificationStatus  map[string]int `json:"by_verification_status"`
	ContextBundleFailures int            `json:"context_bundle_failures"`
	ApplyFailures         int            `json:"apply_failures"`
	Skipped               bool           `json:"skipped"`
	SkipReason            string         `json:"skip_reason,omitempty"`
}

type verifierAgentOutput struct {
	Status                 string
	EvidenceSummary        string
	CounterEvidenceSummary string
	Evidence               []verifierAgentEvidence
	Diagnostics            []string
}

type verifierAgentDocument struct {
	Event                  string                  `json:"event"`
	Status                 string                  `json:"status"`
	VerificationStatus     string                  `json:"verification_status"`
	Verdict                string                  `json:"verdict"`
	Summary                string                  `json:"summary"`
	EvidenceSummary        string                  `json:"evidence_summary"`
	CounterEvidenceSummary string                  `json:"counter_evidence_summary"`
	Evidence               []verifierAgentEvidence `json:"evidence"`
	EvidenceItems          []verifierAgentEvidence `json:"evidence_items"`
	Items                  []verifierAgentEvidence `json:"items"`
	Verification           *verifierAgentDocument  `json:"verification"`
	Result                 *verifierAgentDocument  `json:"result"`
}

type verifierAgentEvidence struct {
	Kind       string          `json:"kind"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary"`
	Path       string          `json:"path"`
	StartLine  int64           `json:"start_line"`
	EndLine    int64           `json:"end_line"`
	Confidence float64         `json:"confidence"`
	Metadata   json.RawMessage `json:"metadata"`
}

func (s *Service) runVerifierAgents(ctx context.Context, session dbgen.ReviewSession, repository dbgen.Repository) (verifierAgentSummary, error) {
	summary := verifierAgentSummary{ByVerificationStatus: map[string]int{}}
	if s.AgentManager == nil || s.ContextBuilder == nil || s.Artifacts == nil {
		summary.Skipped = true
		summary.SkipReason = "verifier runner dependencies are not configured"
		return summary, nil
	}
	configs, err := s.enabledVerifierCLIConfigs(ctx)
	if err != nil {
		return summary, err
	}
	summary.Configured = len(configs)
	if len(configs) == 0 {
		summary.Skipped = true
		summary.SkipReason = "no enabled CLI verifier agent configs"
		return summary, nil
	}
	workspace, err := s.Queries.GetWorkspace(ctx, session.WorkspaceID)
	if err != nil {
		return summary, fmt.Errorf("read workspace for verifier agent: %w", err)
	}
	findings, err := s.Queries.ListFindingsBySession(ctx, session.ID)
	if err != nil {
		return summary, fmt.Errorf("list findings for verifier agent: %w", err)
	}
	findings = prioritizedVerifierFindings(findings, defaultVerifierFindingLimit)
	summary.FindingsEligible = len(findings)
	if len(findings) == 0 {
		summary.Skipped = true
		summary.SkipReason = "no verifier-eligible findings"
		return summary, nil
	}

	configs = boundedAgentConfigs(configs, defaultVerifierAgentLimit)
	for _, finding := range findings {
		summary.FindingsAttempted++
		for _, config := range configs {
			built, err := s.ContextBuilder.BuildFindingContext(ctx, contextbundle.BuildFindingContextParams{
				ReviewSessionID: session.ID,
				FindingID:       finding.ID,
				AgentConfigID:   config.ID,
				Persist:         true,
			})
			if err != nil {
				summary.ContextBundleFailures++
				if eventErr := s.appendVerifierWarning(ctx, session.ID, "", "VerifierContextBuildFailed", map[string]any{
					"finding_id":      finding.ID,
					"agent_config_id": config.ID,
					"error":           err.Error(),
				}); eventErr != nil {
					return summary, eventErr
				}
				continue
			}
			if err := s.appendEvent(ctx, appendEventParams{
				ReviewSessionID: session.ID,
				Type:            "ContextBundleCreated",
				ArtifactID:      nullableEventString(built.Bundle.ArtifactID),
				Payload: map[string]any{
					"phase":             PhaseVerifyFindings,
					"scope":             string(contextbundle.ScopeFinding),
					"finding_id":        finding.ID,
					"agent_config_id":   config.ID,
					"context_bundle_id": built.Bundle.ID,
					"item_count":        built.Bundle.ItemCount,
					"token_estimate":    built.Bundle.TokenEstimate,
					"warnings":          built.Warnings,
				},
			}); err != nil {
				return summary, err
			}
			result, err := s.runVerifierAgent(ctx, session, repository, workspace, config, finding, built.Bundle)
			if err != nil {
				summary.RunsFailed++
				if eventErr := s.appendVerifierWarning(ctx, session.ID, result.Run.ID, "VerifierAgentRunFailed", map[string]any{
					"finding_id":      finding.ID,
					"agent_config_id": config.ID,
					"error":           err.Error(),
				}); eventErr != nil {
					return summary, eventErr
				}
				continue
			}
			summary.RunsStarted++
			if result.Run.Status != agentrun.RunStatusSucceeded {
				summary.RunsFailed++
				continue
			}
			summary.RunsSucceeded++
			applied, err := s.applyVerifierAgentOutput(ctx, finding, config, result)
			if err != nil {
				summary.ApplyFailures++
				if eventErr := s.appendVerifierWarning(ctx, session.ID, result.Run.ID, "VerifierAgentOutputIgnored", map[string]any{
					"finding_id":      finding.ID,
					"agent_config_id": config.ID,
					"error":           err.Error(),
				}); eventErr != nil {
					return summary, eventErr
				}
				continue
			}
			summary.EvidenceItemsCreated += applied.evidenceItemsCreated
			if applied.statusUpdated {
				summary.StatusUpdates++
				summary.ByVerificationStatus[applied.status]++
				finding.VerificationStatus = applied.status
				finding.EvidenceSummary = nullableString(applied.evidenceSummary)
				finding.CounterEvidenceSummary = nullableString(applied.counterEvidenceSummary)
			}
		}
	}
	return summary, nil
}

func (s *Service) enabledVerifierCLIConfigs(ctx context.Context) ([]dbgen.AgentConfig, error) {
	configs, err := s.Queries.ListAgentConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list verifier agent configs: %w", err)
	}
	enabled := make([]dbgen.AgentConfig, 0, len(configs))
	for _, config := range configs {
		if config.Enabled == 0 || agents.AdapterKind(config.AdapterKind) != agents.AdapterCLINonInteractive {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(config.Role))
		if role == "verifier" || strings.Contains(role, "verifier") {
			enabled = append(enabled, config)
		}
	}
	return enabled, nil
}

func prioritizedVerifierFindings(findings []dbgen.Finding, limit int) []dbgen.Finding {
	eligible := make([]dbgen.Finding, 0, len(findings))
	for _, finding := range findings {
		switch finding.VerificationStatus {
		case evidence.StatusDuplicate, evidence.StatusNotActionable:
			continue
		default:
			eligible = append(eligible, finding)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left := verifierStatusPriority(eligible[i].VerificationStatus)
		right := verifierStatusPriority(eligible[j].VerificationStatus)
		if left != right {
			return left < right
		}
		if eligible[i].Severity != eligible[j].Severity {
			return severityPriority(eligible[i].Severity) < severityPriority(eligible[j].Severity)
		}
		if eligible[i].Confidence != eligible[j].Confidence {
			return eligible[i].Confidence > eligible[j].Confidence
		}
		return eligible[i].ID < eligible[j].ID
	})
	if limit > 0 && len(eligible) > limit {
		return eligible[:limit]
	}
	return eligible
}

func verifierStatusPriority(status string) int {
	switch status {
	case evidence.StatusNeedsHuman:
		return 0
	case evidence.StatusPlausible:
		return 1
	case evidence.StatusLikelyFalsePositive:
		return 2
	case evidence.StatusUnverified:
		return 3
	case evidence.StatusVerified:
		return 4
	default:
		return 5
	}
}

func severityPriority(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	case "info", "informational":
		return 4
	default:
		return 5
	}
}

func boundedAgentConfigs(configs []dbgen.AgentConfig, limit int) []dbgen.AgentConfig {
	if limit <= 0 || len(configs) <= limit {
		return configs
	}
	return configs[:limit]
}

func (s *Service) runVerifierAgent(ctx context.Context, session dbgen.ReviewSession, repository dbgen.Repository, workspace dbgen.Workspace, config dbgen.AgentConfig, finding dbgen.Finding, bundle contextbundle.Bundle) (agentrun.RunResult, error) {
	item := runContext{
		Session:     session,
		Repository:  repository,
		Workspace:   workspace,
		AgentConfig: config,
		Bundle:      bundle,
	}
	connectionConfig, limits, err := s.connectionConfig(item)
	if err != nil {
		return agentrun.RunResult{}, err
	}
	capabilities, err := agentCapabilities(config)
	if err != nil {
		return agentrun.RunResult{}, err
	}
	task := agents.AgentTask{
		ID:               s.newID("agent_task_"),
		RunID:            s.newID("agent_run_"),
		ReviewSessionID:  session.ID,
		AgentConfigID:    config.ID,
		ContextBundleID:  bundle.ID,
		Role:             "verifier",
		Prompt:           s.verifierPrompt(session, finding, bundle),
		ContextArtifacts: contextArtifactRefs(bundle),
		RepositoryRoot:   repository.LocalPath,
		WorkspaceRoot:    workspace.RootPath,
		Limits:           limits,
		Metadata: map[string]any{
			"finding_id":        finding.ID,
			"context_bundle_id": bundle.ID,
			"context_scope":     string(contextbundle.ScopeFinding),
		},
	}
	reviewDeadline := time.Time{}
	if session.RuntimeLimitSeconds > 0 && session.StartedAt.Valid {
		if startedAt, err := time.Parse(time.RFC3339Nano, session.StartedAt.String); err == nil {
			reviewDeadline = startedAt.Add(time.Duration(session.RuntimeLimitSeconds) * time.Second)
		}
	}
	result, err := s.AgentManager.Execute(ctx, agentrun.RunParams{
		WorkspaceID:  workspace.ID,
		Config:       connectionConfig,
		Capabilities: capabilities,
		Permissions:  agents.ReviewModePermissionPolicy(),
		Task:         task,
		TimeoutPolicy: agentrun.TimeoutPolicy{
			AgentTimeoutSeconds:  limits.TimeoutSeconds,
			ReviewDeadline:       reviewDeadline,
			ReviewTimeoutSeconds: maxInt64(0, session.RuntimeLimitSeconds),
		},
		Metadata: map[string]any{
			"phase":                 PhaseVerifyFindings,
			"context_bundle_id":     bundle.ID,
			"context_scope":         string(contextbundle.ScopeFinding),
			"finding_id":            finding.ID,
			"output_mode":           string(agents.OutputMode(config.OutputMode)),
			"verifier_agent_config": config.ID,
		},
	})
	if err != nil {
		return result, err
	}
	if err := s.appendAgentRunEvents(ctx, session.ID, result.Events); err != nil {
		return result, err
	}
	if result.Run.Status == agentrun.RunStatusSucceeded {
		if err := s.parseAgentOutputForPhase(ctx, item, &result, PhaseVerifyFindings); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Service) verifierPrompt(session dbgen.ReviewSession, finding dbgen.Finding, bundle contextbundle.Bundle) string {
	var builder strings.Builder
	builder.WriteString("# Role\n\n")
	builder.WriteString("You are a verifier agent inside cocode. Verify one existing finding against the provided scoped context.\n\n")
	builder.WriteString("# Output Contract\n\n")
	builder.WriteString("Return one JSON object with this shape:\n\n")
	builder.WriteString(`{"verification_status":"verified|plausible|needs_human|likely_false_positive|not_actionable","evidence_summary":"short support summary","counter_evidence_summary":"short contradiction or uncertainty summary","evidence":[{"kind":"supporting|counter|missing|test|search|agent|static_analysis","title":"short title","summary":"what this proves","path":"optional/path","start_line":1,"end_line":1,"confidence":0.0}]}`)
	builder.WriteString("\n\n")
	builder.WriteString("# Rules\n\n")
	builder.WriteString("- Verify only the finding below; do not create unrelated review findings.\n")
	builder.WriteString("- Prefer cited code evidence over speculation.\n")
	builder.WriteString("- Use `needs_human` when the scoped context is insufficient.\n")
	builder.WriteString("- Keep evidence items bounded and cite paths/lines when available.\n")
	builder.WriteString("- Do not edit files.\n\n")
	builder.WriteString("# Finding\n\n")
	builder.WriteString("Review session ID: ")
	builder.WriteString(session.ID)
	builder.WriteByte('\n')
	builder.WriteString("Finding ID: ")
	builder.WriteString(finding.ID)
	builder.WriteByte('\n')
	builder.WriteString("Claim: ")
	builder.WriteString(finding.CanonicalClaim)
	builder.WriteByte('\n')
	builder.WriteString("Category: ")
	builder.WriteString(finding.Category)
	builder.WriteByte('\n')
	builder.WriteString("Severity: ")
	builder.WriteString(finding.Severity)
	builder.WriteByte('\n')
	builder.WriteString("Current verification status: ")
	builder.WriteString(finding.VerificationStatus)
	builder.WriteByte('\n')
	if finding.PrimaryPath.Valid {
		builder.WriteString("Primary path: ")
		builder.WriteString(finding.PrimaryPath.String)
		if finding.PrimaryStartLine.Valid {
			builder.WriteString(":")
			builder.WriteString(fmt.Sprint(finding.PrimaryStartLine.Int64))
			if finding.PrimaryEndLine.Valid && finding.PrimaryEndLine.Int64 != finding.PrimaryStartLine.Int64 {
				builder.WriteString("-")
				builder.WriteString(fmt.Sprint(finding.PrimaryEndLine.Int64))
			}
		}
		builder.WriteByte('\n')
	}
	if finding.EvidenceSummary.Valid && strings.TrimSpace(finding.EvidenceSummary.String) != "" {
		builder.WriteString("Local evidence summary: ")
		builder.WriteString(strings.TrimSpace(finding.EvidenceSummary.String))
		builder.WriteByte('\n')
	}
	if finding.CounterEvidenceSummary.Valid && strings.TrimSpace(finding.CounterEvidenceSummary.String) != "" {
		builder.WriteString("Local counter-evidence summary: ")
		builder.WriteString(strings.TrimSpace(finding.CounterEvidenceSummary.String))
		builder.WriteByte('\n')
	}
	builder.WriteString("\n")
	builder.WriteString(contextbundle.RenderBundle(bundle))
	return builder.String()
}

type verifierApplyResult struct {
	evidenceItemsCreated   int
	statusUpdated          bool
	status                 string
	evidenceSummary        string
	counterEvidenceSummary string
}

func (s *Service) applyVerifierAgentOutput(ctx context.Context, finding dbgen.Finding, config dbgen.AgentConfig, result agentrun.RunResult) (verifierApplyResult, error) {
	if !result.Run.ParsedOutputArtifactID.Valid {
		return verifierApplyResult{}, nil
	}
	content, _, err := s.Artifacts.Read(ctx, result.Run.ParsedOutputArtifactID.String)
	if err != nil {
		return verifierApplyResult{}, fmt.Errorf("read verifier parsed output: %w", err)
	}
	var parsed agentoutput.ParsedOutput
	if err := json.Unmarshal(content, &parsed); err != nil {
		return verifierApplyResult{}, fmt.Errorf("decode verifier parsed output: %w", err)
	}
	output := parseVerifierAgentOutput(parsed)
	if len(output.Diagnostics) > 0 && len(output.Evidence) == 0 && output.Status == "" {
		return verifierApplyResult{}, fmt.Errorf("verifier output had no usable verification result: %s", strings.Join(output.Diagnostics, "; "))
	}
	applied := verifierApplyResult{
		status:                 finding.VerificationStatus,
		evidenceSummary:        nullableSQLStringValue(finding.EvidenceSummary),
		counterEvidenceSummary: nullableSQLStringValue(finding.CounterEvidenceSummary),
	}
	if trimmed := strings.TrimSpace(output.EvidenceSummary); trimmed != "" {
		applied.evidenceSummary = truncateString(trimmed, defaultVerifierTextSummaryBytes)
	}
	if trimmed := strings.TrimSpace(output.CounterEvidenceSummary); trimmed != "" {
		applied.counterEvidenceSummary = truncateString(trimmed, defaultVerifierTextSummaryBytes)
	}
	for index, item := range output.Evidence {
		if index >= defaultVerifierEvidenceItemLimit {
			break
		}
		if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Summary) == "" {
			continue
		}
		title := strings.TrimSpace(item.Title)
		summary := strings.TrimSpace(item.Summary)
		if title == "" {
			title = "Verifier agent evidence"
		}
		if summary == "" {
			summary = title
		}
		if _, err := s.Queries.CreateEvidenceItem(ctx, dbgen.CreateEvidenceItemParams{
			ID:           s.newID("evidence_item_"),
			FindingID:    finding.ID,
			Kind:         normalizeVerifierEvidenceKind(item.Kind),
			Title:        truncateString(title, 240),
			Summary:      truncateString(summary, defaultVerifierTextSummaryBytes),
			Path:         nullableString(item.Path),
			StartLine:    nullablePositiveInt64(item.StartLine),
			EndLine:      nullablePositiveInt64(normalizeEndLine(item.StartLine, item.EndLine)),
			Confidence:   clampVerifierConfidence(item.Confidence),
			MetadataJson: verifierEvidenceMetadata(config, result.Run, item.Metadata),
			CreatedAt:    s.now().Format(time.RFC3339Nano),
		}); err != nil {
			return applied, fmt.Errorf("create verifier evidence item: %w", err)
		}
		applied.evidenceItemsCreated++
	}
	if output.Status != "" {
		applied.status = output.Status
	}
	if output.Status != "" || output.EvidenceSummary != "" || output.CounterEvidenceSummary != "" {
		updated, err := s.Queries.UpdateFindingVerificationEvidence(ctx, dbgen.UpdateFindingVerificationEvidenceParams{
			VerificationStatus:     applied.status,
			EvidenceSummary:        nullableString(applied.evidenceSummary),
			CounterEvidenceSummary: nullableString(applied.counterEvidenceSummary),
			UpdatedAt:              s.now().Format(time.RFC3339Nano),
			ID:                     finding.ID,
		})
		if err != nil {
			return applied, fmt.Errorf("update finding from verifier output: %w", err)
		}
		applied.statusUpdated = true
		applied.status = updated.VerificationStatus
		applied.evidenceSummary = nullableSQLStringValue(updated.EvidenceSummary)
		applied.counterEvidenceSummary = nullableSQLStringValue(updated.CounterEvidenceSummary)
	}
	return applied, nil
}

func parseVerifierAgentOutput(parsed agentoutput.ParsedOutput) verifierAgentOutput {
	output := verifierAgentOutput{}
	for _, raw := range parsed.Documents {
		doc, err := decodeVerifierAgentDocument(raw)
		if err != nil {
			output.Diagnostics = append(output.Diagnostics, err.Error())
			continue
		}
		output.merge(doc)
	}
	if len(parsed.Documents) == 0 && strings.TrimSpace(parsed.Text) != "" {
		output.Evidence = append(output.Evidence, verifierAgentEvidence{
			Kind:       evidence.KindAgent,
			Title:      "Verifier agent response",
			Summary:    truncateString(strings.TrimSpace(parsed.Text), defaultVerifierTextSummaryBytes),
			Confidence: 0.5,
		})
	}
	for _, diagnostic := range parsed.Diagnostics {
		output.Diagnostics = append(output.Diagnostics, diagnostic.Message)
	}
	return output
}

func decodeVerifierAgentDocument(raw json.RawMessage) (verifierAgentDocument, error) {
	var doc verifierAgentDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return verifierAgentDocument{}, fmt.Errorf("decode verifier document: %w", err)
	}
	if doc.Verification != nil {
		return *doc.Verification, nil
	}
	if doc.Result != nil {
		return *doc.Result, nil
	}
	return doc, nil
}

func (output *verifierAgentOutput) merge(doc verifierAgentDocument) {
	if status := normalizeVerifierStatus(firstNonEmpty(doc.VerificationStatus, doc.Status, doc.Verdict)); status != "" {
		output.Status = status
	} else if firstNonEmpty(doc.VerificationStatus, doc.Status, doc.Verdict) != "" {
		output.Diagnostics = append(output.Diagnostics, "unsupported verifier status "+firstNonEmpty(doc.VerificationStatus, doc.Status, doc.Verdict))
	}
	if summary := strings.TrimSpace(firstNonEmpty(doc.EvidenceSummary, doc.Summary)); summary != "" {
		output.EvidenceSummary = summary
	}
	if summary := strings.TrimSpace(doc.CounterEvidenceSummary); summary != "" {
		output.CounterEvidenceSummary = summary
	}
	output.Evidence = append(output.Evidence, doc.Evidence...)
	output.Evidence = append(output.Evidence, doc.EvidenceItems...)
	output.Evidence = append(output.Evidence, doc.Items...)
}

func normalizeVerifierStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	status = strings.ReplaceAll(status, "-", "_")
	status = strings.ReplaceAll(status, " ", "_")
	switch status {
	case evidence.StatusVerified, "supported", "confirm", "confirmed", "true_positive":
		return evidence.StatusVerified
	case evidence.StatusPlausible, "partially_supported":
		return evidence.StatusPlausible
	case evidence.StatusNeedsHuman, "needs_review", "uncertain", "unknown", "inconclusive":
		return evidence.StatusNeedsHuman
	case evidence.StatusLikelyFalsePositive, "false_positive", "not_supported", "unsupported", "contradicted":
		return evidence.StatusLikelyFalsePositive
	case evidence.StatusNotActionable, "invalid", "out_of_scope":
		return evidence.StatusNotActionable
	default:
		return ""
	}
}

func normalizeVerifierEvidenceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case evidence.KindSupporting, "support", "supports":
		return evidence.KindSupporting
	case evidence.KindCounter, "counter_evidence", "contradiction", "contradicts":
		return evidence.KindCounter
	case evidence.KindMissing, "missing_evidence":
		return evidence.KindMissing
	case evidence.KindTest, "tests":
		return evidence.KindTest
	case evidence.KindSearch, "code_search":
		return evidence.KindSearch
	case evidence.KindStaticAnalysis, "static":
		return evidence.KindStaticAnalysis
	default:
		return evidence.KindAgent
	}
}

func verifierEvidenceMetadata(config dbgen.AgentConfig, run dbgen.AgentRun, raw json.RawMessage) string {
	metadata := map[string]any{
		"producer":        "verifier_agent",
		"source":          "cli_verifier",
		"agent_config_id": config.ID,
		"agent_run_id":    run.ID,
	}
	if len(raw) > 0 && json.Valid(raw) {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err == nil {
			metadata["agent_metadata"] = decoded
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return `{"producer":"verifier_agent","source":"cli_verifier"}`
	}
	return string(encoded)
}

func normalizeEndLine(startLine int64, endLine int64) int64 {
	if startLine > 0 && endLine < startLine {
		return startLine
	}
	return endLine
}

func clampVerifierConfidence(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	case value == 0:
		return 0.5
	default:
		return value
	}
}

func nullableSQLStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func truncateString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Service) appendVerifierWarning(ctx context.Context, reviewSessionID string, agentRunID string, eventType string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["phase"] = PhaseVerifyFindings
	return s.appendEvent(ctx, appendEventParams{
		ReviewSessionID: reviewSessionID,
		AgentRunID:      nullableEventString(agentRunID),
		Type:            eventType,
		Level:           "warn",
		Payload:         payload,
	})
}
