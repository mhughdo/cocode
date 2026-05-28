package evidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/codeintel"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	GraphStatusReady   = "ready"
	GraphStatusPartial = "partial"
)

const (
	mapSourceContextLines    = 3
	mapSourceMaxSnippetBytes = 8 * 1024
	mapSourceMaxFileBytes    = 256 * 1024
)

const (
	NodeChangedCode     = "changed_code"
	NodeEntrypoint      = "entrypoint"
	NodeRoute           = "route"
	NodeRelatedCode     = "related_code"
	NodeMiddleware      = "middleware"
	NodeGuard           = "guard"
	NodeHandler         = "handler"
	NodeTest            = "test"
	NodeConfig          = "config"
	NodeCounterEvidence = "counter_evidence"
	NodeMissingGuard    = "missing_guard"
	NodeUnknown         = "unknown"
)

const (
	EdgeCalls        = "calls"
	EdgeMounts       = "mounts"
	EdgeProtects     = "protects"
	EdgeTests        = "tests"
	EdgeSupports     = "supports"
	EdgeContradicts  = "contradicts"
	EdgeMissingGuard = "missing_guard"
	EdgeUnknown      = "unknown"
)

const (
	EdgeStatusObserved = "observed"
	EdgeStatusMissing  = "missing"
)

const (
	defaultEvidenceMapItemLimit = 80
	defaultCallPathStepLimit    = 8
	goplsCallHierarchyLimit     = 8
	goplsCallHierarchyTimeout   = 8 * time.Second
)

type MapSummary struct {
	Findings int            `json:"findings"`
	Ready    int            `json:"ready"`
	Partial  int            `json:"partial"`
	Nodes    int            `json:"nodes"`
	Edges    int            `json:"edges"`
	ByStatus map[string]int `json:"by_status"`
}

type MapView struct {
	Graph                     GraphView          `json:"graph"`
	Finding                   MapFindingView     `json:"finding"`
	Hierarchy                 []HierarchyItem    `json:"hierarchy"`
	Nodes                     []NodeView         `json:"nodes"`
	Edges                     []EdgeView         `json:"edges"`
	CallPath                  []CallPathStepView `json:"call_path"`
	CallPaths                 []CallPathView     `json:"call_paths"`
	CallPathUnavailableReason string             `json:"call_path_unavailable_reason,omitempty"`
	Legend                    []LegendItem       `json:"legend"`
	Panel                     MapPanel           `json:"panel"`
	MissingReasons            []string           `json:"missing_reasons,omitempty"`
}

type MapContext struct {
	RepositoryLocalPath string
}

type GraphView struct {
	ID              string          `json:"id"`
	FindingID       string          `json:"finding_id"`
	ReviewSessionID string          `json:"review_session_id"`
	Status          string          `json:"status"`
	Summary         string          `json:"summary,omitempty"`
	Layout          json.RawMessage `json:"layout"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

type MapFindingView struct {
	ID                     string  `json:"id"`
	ReviewSessionID        string  `json:"review_session_id"`
	CanonicalClaim         string  `json:"canonical_claim"`
	Category               string  `json:"category"`
	Severity               string  `json:"severity"`
	Confidence             float64 `json:"confidence"`
	VerificationStatus     string  `json:"verification_status"`
	DecisionStatus         string  `json:"decision_status"`
	PrimaryPath            string  `json:"primary_path,omitempty"`
	PrimaryStartLine       int64   `json:"primary_start_line,omitempty"`
	PrimaryEndLine         int64   `json:"primary_end_line,omitempty"`
	EvidenceSummary        string  `json:"evidence_summary,omitempty"`
	CounterEvidenceSummary string  `json:"counter_evidence_summary,omitempty"`
	SuggestedFix           string  `json:"suggested_fix,omitempty"`
}

type HierarchyItem struct {
	Path            string   `json:"path"`
	Kind            string   `json:"kind"`
	StartLine       int64    `json:"start_line,omitempty"`
	EndLine         int64    `json:"end_line,omitempty"`
	NodeIDs         []string `json:"node_ids"`
	EvidenceItemIDs []string `json:"evidence_item_ids,omitempty"`
}

type NodeView struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Label          string          `json:"label"`
	Explanation    string          `json:"explanation,omitempty"`
	Path           string          `json:"path,omitempty"`
	Symbol         string          `json:"symbol,omitempty"`
	StartLine      int64           `json:"start_line,omitempty"`
	EndLine        int64           `json:"end_line,omitempty"`
	EvidenceItemID string          `json:"evidence_item_id,omitempty"`
	Confidence     float64         `json:"confidence"`
	DeepLink       *NodeDeepLink   `json:"deep_link,omitempty"`
	CodeSnippet    string          `json:"code_snippet,omitempty"`
	LineWindow     *SourceWindow   `json:"line_window,omitempty"`
	FileContent    string          `json:"file_content,omitempty"`
	FileLineCount  int64           `json:"file_line_count,omitempty"`
	FileTruncated  bool            `json:"file_truncated,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
}

type NodeDeepLink struct {
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	StartLine int64  `json:"start_line,omitempty"`
	EndLine   int64  `json:"end_line,omitempty"`
}

type SourceWindow struct {
	StartLine int64 `json:"start_line"`
	EndLine   int64 `json:"end_line"`
}

type EdgeView struct {
	ID          string          `json:"id"`
	Source      string          `json:"source"`
	Target      string          `json:"target"`
	Kind        string          `json:"kind"`
	Status      string          `json:"status"`
	Label       string          `json:"label,omitempty"`
	Explanation string          `json:"explanation,omitempty"`
	Confidence  float64         `json:"confidence"`
	Metadata    json.RawMessage `json:"metadata"`
}

type CallPathView struct {
	ID         string             `json:"id"`
	Label      string             `json:"label,omitempty"`
	Summary    string             `json:"summary,omitempty"`
	Confidence float64            `json:"confidence"`
	Steps      []CallPathStepView `json:"steps"`
}

type CallPathStepView struct {
	ID          string `json:"id,omitempty"`
	NodeID      string `json:"node_id,omitempty"`
	StepIndex   int64  `json:"step_index"`
	Path        string `json:"path,omitempty"`
	StartLine   int64  `json:"start_line,omitempty"`
	EndLine     int64  `json:"end_line,omitempty"`
	Label       string `json:"label"`
	Explanation string `json:"explanation,omitempty"`
}

type LegendItem struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type MapPanel struct {
	Claim                  string                `json:"claim"`
	Severity               string                `json:"severity"`
	VerificationStatus     string                `json:"verification_status"`
	DecisionStatus         string                `json:"decision_status"`
	ConnectionSummary      string                `json:"connection_summary,omitempty"`
	EvidenceSummary        string                `json:"evidence_summary,omitempty"`
	CounterEvidenceSummary string                `json:"counter_evidence_summary,omitempty"`
	SuggestedFix           string                `json:"suggested_fix,omitempty"`
	EvidenceCounts         map[string]int        `json:"evidence_counts"`
	Evidence               []MapPanelEvidenceRef `json:"evidence"`
}

type MapPanelEvidenceRef struct {
	ID            string        `json:"id"`
	Kind          string        `json:"kind"`
	Title         string        `json:"title"`
	Summary       string        `json:"summary"`
	Path          string        `json:"path,omitempty"`
	StartLine     int64         `json:"start_line,omitempty"`
	EndLine       int64         `json:"end_line,omitempty"`
	Confidence    float64       `json:"confidence"`
	CodeSnippet   string        `json:"code_snippet,omitempty"`
	LineWindow    *SourceWindow `json:"line_window,omitempty"`
	FileContent   string        `json:"file_content,omitempty"`
	FileLineCount int64         `json:"file_line_count,omitempty"`
	FileTruncated bool          `json:"file_truncated,omitempty"`
}

type mapBuildPlan struct {
	status                    string
	summary                   string
	layout                    map[string]any
	missingReasons            []string
	callPathUnavailableReason string
	callHierarchyAttempted    bool
	callHierarchyAvailable    bool
	callHierarchySource       string
	callHierarchyTarget       string
	callHierarchyReason       string
	connectionSummary         string
	nodes                     []nodeSpec
	edges                     []edgeSpec
	callSteps                 []callStepSpec
	primaryNodeKey            string
	omittedEvidenceItems      int
}

type nodeSpec struct {
	key            string
	kind           string
	label          string
	path           string
	symbol         string
	startLine      int64
	endLine        int64
	evidenceItemID string
	confidence     float64
	metadata       map[string]any
}

type edgeSpec struct {
	key        string
	sourceKey  string
	targetKey  string
	kind       string
	status     string
	label      string
	confidence float64
	metadata   map[string]any
}

type callStepSpec struct {
	nodeKey   string
	path      string
	startLine int64
	endLine   int64
	label     string
}

func (s *Service) BuildSessionEvidenceMaps(ctx context.Context, session dbgen.ReviewSession) (MapSummary, error) {
	if s == nil || s.Queries == nil {
		return MapSummary{}, errors.New("evidence map queries are required")
	}
	findings, err := s.Queries.ListFindingsBySession(ctx, session.ID)
	if err != nil {
		return MapSummary{}, fmt.Errorf("list findings for evidence maps: %w", err)
	}
	mapCtx := MapContext{}
	if session.RepositoryID != "" {
		if repository, err := s.Queries.GetRepository(ctx, session.RepositoryID); err == nil {
			mapCtx.RepositoryLocalPath = repository.LocalPath
		}
	}
	summary := MapSummary{Findings: len(findings), ByStatus: map[string]int{}}
	for _, finding := range findings {
		view, err := s.RebuildEvidenceMapWithContext(ctx, finding, mapCtx)
		if err != nil {
			return MapSummary{}, err
		}
		summary.Nodes += len(view.Nodes)
		summary.Edges += len(view.Edges)
		summary.ByStatus[view.Graph.Status]++
		switch view.Graph.Status {
		case GraphStatusReady:
			summary.Ready++
		case GraphStatusPartial:
			summary.Partial++
		}
	}
	return summary, nil
}

func (s *Service) LoadOrRebuildEvidenceMap(ctx context.Context, finding dbgen.Finding) (MapView, bool, error) {
	if s == nil || s.Queries == nil {
		return MapView{}, false, errors.New("evidence map queries are required")
	}
	graph, err := s.Queries.GetEvidenceGraphByFinding(ctx, finding.ID)
	if err == nil && !evidenceGraphStaleForFinding(graph, finding) {
		view, err := s.mapView(ctx, finding, graph)
		if err != nil {
			return MapView{}, false, err
		}
		return view, false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MapView{}, false, err
	}
	view, err := s.RebuildEvidenceMap(ctx, finding)
	if err != nil {
		return MapView{}, false, err
	}
	return view, true, nil
}

func evidenceGraphStaleForFinding(graph dbgen.EvidenceGraph, finding dbgen.Finding) bool {
	graphUpdated, graphErr := time.Parse(time.RFC3339Nano, graph.UpdatedAt)
	findingUpdated, findingErr := time.Parse(time.RFC3339Nano, finding.UpdatedAt)
	if graphErr != nil || findingErr != nil {
		return false
	}
	return graphUpdated.Before(findingUpdated)
}

func (s *Service) LoadEvidenceMap(ctx context.Context, finding dbgen.Finding) (MapView, error) {
	if s == nil || s.Queries == nil {
		return MapView{}, errors.New("evidence map queries are required")
	}
	graph, err := s.Queries.GetEvidenceGraphByFinding(ctx, finding.ID)
	if err != nil {
		return MapView{}, err
	}
	return s.mapView(ctx, finding, graph)
}

func (s *Service) RebuildEvidenceMap(ctx context.Context, finding dbgen.Finding) (MapView, error) {
	return s.RebuildEvidenceMapWithContext(ctx, finding, MapContext{})
}

func (s *Service) RebuildEvidenceMapWithContext(ctx context.Context, finding dbgen.Finding, mapCtx MapContext) (MapView, error) {
	if s == nil || s.Queries == nil {
		return MapView{}, errors.New("evidence map queries are required")
	}
	items, err := s.Queries.ListEvidenceItemsByFinding(ctx, finding.ID)
	if err != nil {
		return MapView{}, fmt.Errorf("list evidence for map %s: %w", finding.ID, err)
	}
	if existing, err := s.Queries.GetEvidenceGraphByFinding(ctx, finding.ID); err == nil {
		if err := s.Queries.DeleteEvidenceGraph(ctx, existing.ID); err != nil {
			return MapView{}, fmt.Errorf("delete previous evidence map %s: %w", existing.ID, err)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return MapView{}, fmt.Errorf("read previous evidence map %s: %w", finding.ID, err)
	}

	plan := s.buildMapPlan(ctx, finding, items, mapCtx)
	now := s.now().Format(time.RFC3339Nano)
	layout := mustMetadata(plan.layout)
	graph, err := s.Queries.CreateEvidenceGraph(ctx, dbgen.CreateEvidenceGraphParams{
		ID:              stableMapID("evidence_graph_", finding.ID),
		FindingID:       finding.ID,
		ReviewSessionID: finding.ReviewSessionID,
		Status:          plan.status,
		LayoutJson:      string(layout),
		Summary:         nullableString(plan.summary),
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return MapView{}, fmt.Errorf("create evidence map graph for finding %s: %w", finding.ID, err)
	}

	nodeIDs := make(map[string]string, len(plan.nodes))
	for _, spec := range plan.nodes {
		node, err := s.Queries.CreateEvidenceNode(ctx, dbgen.CreateEvidenceNodeParams{
			ID:              stableMapID("evidence_node_", finding.ID, spec.key),
			EvidenceGraphID: graph.ID,
			Kind:            normalizeNodeKind(spec.kind),
			Label:           trimOrDefault(spec.label, "Evidence node"),
			Path:            nullableString(spec.path),
			Symbol:          nullableString(spec.symbol),
			StartLine:       nullablePositiveInt64(spec.startLine),
			EndLine:         nullablePositiveInt64(spec.endLine),
			EvidenceItemID:  nullableString(spec.evidenceItemID),
			Confidence:      clampConfidence(spec.confidence),
			MetadataJson:    string(mustMetadata(spec.metadata)),
		})
		if err != nil {
			return MapView{}, fmt.Errorf("create evidence map node %s: %w", spec.key, err)
		}
		nodeIDs[spec.key] = node.ID
	}
	for _, spec := range plan.edges {
		sourceID, ok := nodeIDs[spec.sourceKey]
		if !ok {
			continue
		}
		targetID, ok := nodeIDs[spec.targetKey]
		if !ok {
			continue
		}
		if _, err := s.Queries.CreateEvidenceEdge(ctx, dbgen.CreateEvidenceEdgeParams{
			ID:              stableMapID("evidence_edge_", finding.ID, spec.key),
			EvidenceGraphID: graph.ID,
			SourceNodeID:    sourceID,
			TargetNodeID:    targetID,
			Kind:            normalizeEdgeKind(spec.kind),
			Status:          normalizeEdgeStatus(spec.status),
			Label:           nullableString(spec.label),
			Confidence:      clampConfidence(spec.confidence),
			MetadataJson:    string(mustMetadata(spec.metadata)),
		}); err != nil {
			return MapView{}, fmt.Errorf("create evidence map edge %s: %w", spec.key, err)
		}
	}
	if len(plan.callSteps) > 0 {
		path, err := s.Queries.CreateCallPath(ctx, dbgen.CreateCallPathParams{
			ID:              stableMapID("call_path_", finding.ID, "primary"),
			EvidenceGraphID: graph.ID,
			Label:           nullableString("Primary evidence path"),
			Confidence:      0.7,
			CreatedAt:       now,
		})
		if err != nil {
			return MapView{}, fmt.Errorf("create evidence map call path: %w", err)
		}
		for index, step := range plan.callSteps {
			if _, err := s.Queries.CreateCallPathStep(ctx, dbgen.CreateCallPathStepParams{
				ID:         stableMapID("call_path_step_", finding.ID, fmt.Sprint(index), step.nodeKey),
				CallPathID: path.ID,
				StepIndex:  int64(index),
				NodeID:     nullableString(nodeIDs[step.nodeKey]),
				Path:       nullableString(step.path),
				StartLine:  nullablePositiveInt64(step.startLine),
				EndLine:    nullablePositiveInt64(step.endLine),
				Label:      trimOrDefault(step.label, "Evidence step"),
			}); err != nil {
				return MapView{}, fmt.Errorf("create evidence map call path step %d: %w", index, err)
			}
		}
	}
	return s.mapView(ctx, finding, graph)
}

func (s *Service) buildMapPlan(ctx context.Context, finding dbgen.Finding, items []dbgen.EvidenceItem, mapCtx MapContext) mapBuildPlan {
	plan := buildBaseMapPlan(finding, items)
	s.enrichMapPlanWithGopls(ctx, &plan, finding, mapCtx)
	finalizeMapPlan(finding, &plan)
	return plan
}

func buildMapPlan(finding dbgen.Finding, items []dbgen.EvidenceItem) mapBuildPlan {
	plan := buildBaseMapPlan(finding, items)
	finalizeMapPlan(finding, &plan)
	return plan
}

func buildBaseMapPlan(finding dbgen.Finding, items []dbgen.EvidenceItem) mapBuildPlan {
	plan := mapBuildPlan{
		status:         GraphStatusReady,
		primaryNodeKey: "primary",
	}
	primaryEvidence := primaryEvidenceItem(finding, items)
	primary := primaryNodeSpec(finding, primaryEvidence)
	if primary.path == "" {
		plan.missingReasons = append(plan.missingReasons, "Finding has no primary file location.")
		primary.kind = NodeUnknown
		primary.label = "Finding location unavailable"
		primary.metadata["reason"] = "missing_primary_location"
	}
	plan.nodes = append(plan.nodes, primary)

	graphItems, omitted := boundedEvidenceItemsForGraph(items, primaryEvidence)
	plan.omittedEvidenceItems = omitted
	if omitted > 0 {
		plan.missingReasons = append(plan.missingReasons, fmt.Sprintf("%d evidence item(s) omitted from graph to keep the response bounded.", omitted))
	}
	for _, item := range graphItems {
		spec := evidenceNodeSpec(finding, item)
		plan.nodes = append(plan.nodes, spec)
		if edge := evidenceEdgeSpec(item, plan.primaryNodeKey, spec.key); edge.key != "" {
			plan.edges = append(plan.edges, edge)
		}
	}
	if missing := syntheticMissingRelationshipNode(finding, items); missing.key != "" {
		plan.nodes = append(plan.nodes, missing)
		plan.edges = append(plan.edges, edgeSpec{
			key:        "missing_relationship:" + missing.key,
			sourceKey:  plan.primaryNodeKey,
			targetKey:  missing.key,
			kind:       EdgeMissingGuard,
			status:     EdgeStatusMissing,
			label:      "Expected protection is missing",
			confidence: clampConfidence(finding.Confidence),
			metadata: map[string]any{
				"source":      "local_rule",
				"rule":        missing.metadata["rule"],
				"explanation": "The map could not verify the expected protection edge from the issue line to a guard.",
			},
		})
	}
	return plan
}

func finalizeMapPlan(finding dbgen.Finding, plan *mapBuildPlan) {
	if plan == nil {
		return
	}
	plan.status = GraphStatusReady
	plan.callSteps, plan.callPathUnavailableReason = callPathSteps(plan.nodes, plan.edges, plan.primaryNodeKey)
	if plan.callPathUnavailableReason != "" {
		plan.missingReasons = append(plan.missingReasons, plan.callPathUnavailableReason)
	}
	if len(plan.edges) == 0 {
		plan.missingReasons = append(plan.missingReasons, "No supporting, counter, test, or missing relationship evidence was available for graph edges.")
	}
	if len(plan.missingReasons) > 0 {
		plan.status = GraphStatusPartial
	}
	plan.connectionSummary = evidenceMapConnectionSummary(finding, *plan)
	plan.summary = mapSummaryText(finding, *plan)
	plan.layout = map[string]any{
		"direction":                    "LR",
		"generated_by":                 "local_evidence_map_builder",
		"missing_reasons":              plan.missingReasons,
		"call_path_unavailable_reason": plan.callPathUnavailableReason,
		"connection_summary":           plan.connectionSummary,
		"call_hierarchy": map[string]any{
			"source":    trimOrDefault(plan.callHierarchySource, "gopls"),
			"attempted": plan.callHierarchyAttempted,
			"available": plan.callHierarchyAvailable,
			"target":    plan.callHierarchyTarget,
			"reason":    plan.callHierarchyReason,
		},
		"omitted_evidence_items": plan.omittedEvidenceItems,
		"limits": map[string]any{
			"evidence_items":  defaultEvidenceMapItemLimit,
			"call_path_steps": defaultCallPathStepLimit,
		},
	}
}

var (
	goplsHierarchyRelationRE   = regexp.MustCompile(`^(caller|callee)\[\d+\]: .* from/to function (.+) in (.+):(\d+):`)
	goplsHierarchyIdentifierRE = regexp.MustCompile(`^identifier: function (.+) in (.+):(\d+):`)
	lookPathGopls              = exec.LookPath
	runGoplsCallHierarchy      = func(ctx context.Context, goplsPath string, dir string, target string) ([]byte, error) {
		command := exec.CommandContext(ctx, goplsPath, "call_hierarchy", target)
		command.Dir = dir
		return command.CombinedOutput()
	}
)

type goplsHierarchyEntry struct {
	direction string
	symbol    string
	path      string
	line      int64
}

func (s *Service) enrichMapPlanWithGopls(ctx context.Context, plan *mapBuildPlan, finding dbgen.Finding, mapCtx MapContext) {
	if plan == nil {
		return
	}
	repoRoot := strings.TrimSpace(mapCtx.RepositoryLocalPath)
	if repoRoot == "" {
		plan.callHierarchyReason = "repository local path was not available for call-site enrichment"
		return
	}
	primary := mapPlanNode(plan, plan.primaryNodeKey)
	if primary == nil || primary.path == "" || primary.startLine <= 0 {
		plan.callHierarchyReason = "primary location is not a source file with a line anchor"
		return
	}
	language := codeintel.DetectLanguage(primary.path)
	if language == "" {
		plan.callHierarchyReason = "primary location language is not supported for call-site enrichment"
		return
	}
	plan.callHierarchyAttempted = true
	targetPath := primary.path
	if filepath.IsAbs(targetPath) {
		if rel, err := filepath.Rel(repoRoot, targetPath); err == nil {
			targetPath = rel
		}
	}
	targetPath = filepath.ToSlash(targetPath)
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(targetPath))); err != nil {
		plan.callHierarchyReason = "primary source file was not found in the local repository"
		return
	}
	targetLine, targetColumn := callHierarchyTarget(repoRoot, targetPath, primary.startLine)
	var symbol codeintel.Symbol
	if resolved, ok := codeintel.ResolveEnclosingSymbol(repoRoot, targetPath, primary.startLine); ok {
		symbol = resolved
		annotatePrimarySymbol(primary, resolved)
		targetLine = resolved.NameLine
		targetColumn = resolved.NameColumn
	}
	plan.callHierarchyTarget = fmt.Sprintf("%s:%d:%d", targetPath, targetLine, targetColumn)
	if language != "go" {
		if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, fmt.Sprintf("%s LSP call hierarchy is not configured", codeintel.LanguageLabel(language))) {
			return
		}
		plan.callHierarchyReason = fmt.Sprintf("no local %s caller evidence was found", codeintel.LanguageLabel(language))
		return
	}
	goplsPath, err := lookPathGopls("gopls")
	if err != nil {
		if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, "gopls executable was not found") {
			return
		}
		plan.callHierarchyReason = "gopls executable was not found"
		return
	}
	moduleRoot, ok := findGoModuleRoot(repoRoot, targetPath)
	if !ok {
		if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, "no Go module root was found for the primary file") {
			return
		}
		plan.callHierarchyReason = "no Go module root was found for the primary file"
		return
	}
	moduleTargetPath := targetPath
	if rel, err := filepath.Rel(moduleRoot, filepath.Join(repoRoot, filepath.FromSlash(targetPath))); err == nil && !isParentRelativePath(rel) {
		moduleTargetPath = filepath.ToSlash(rel)
	}

	commandCtx, cancel := context.WithTimeout(ctx, goplsCallHierarchyTimeout)
	defer cancel()
	target := fmt.Sprintf("%s:%d:%d", moduleTargetPath, targetLine, targetColumn)
	plan.callHierarchyTarget = target
	output, err := runGoplsCallHierarchy(commandCtx, goplsPath, moduleRoot, target)
	if commandCtx.Err() != nil {
		if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, fmt.Sprintf("gopls call_hierarchy timed out after %s", goplsCallHierarchyTimeout)) {
			return
		}
		plan.callHierarchyReason = fmt.Sprintf("gopls call_hierarchy timed out after %s", goplsCallHierarchyTimeout)
		return
	}
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			reason := "gopls call_hierarchy failed: " + truncateText(detail, 180)
			if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, reason) {
				return
			}
			plan.callHierarchyReason = reason
		} else {
			reason := "gopls call_hierarchy returned no usable result"
			if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, reason) {
				return
			}
			plan.callHierarchyReason = reason
		}
		return
	}
	identifier, entries := parseGoplsCallHierarchy(string(output), repoRoot)
	if identifier != nil {
		primary.symbol = identifier.symbol
		primary.metadata["symbol"] = identifier.symbol
		primary.metadata["gopls_call_hierarchy"] = true
	}
	if len(entries) == 0 {
		if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, "gopls found the symbol but no callers or callees") {
			return
		}
		plan.callHierarchyReason = "gopls found the symbol but no callers or callees"
		return
	}
	entries = selectGoplsHierarchyEntries(entries, goplsCallHierarchyLimit)
	if len(entries) == 0 {
		if enrichMapPlanWithLocalCallers(plan, repoRoot, symbol, "gopls found only duplicate caller/callee entries") {
			return
		}
		plan.callHierarchyReason = "gopls found only duplicate caller/callee entries"
		return
	}
	plan.callHierarchyAvailable = true
	plan.callHierarchySource = "gopls_call_hierarchy"
	plan.callHierarchyReason = ""
	for _, entry := range entries {
		switch entry.direction {
		case "caller":
			key := fmt.Sprintf("gopls:caller:%s:%d:%s", entry.path, entry.line, entry.symbol)
			plan.nodes = append(plan.nodes, goplsNodeSpec(key, entry, NodeHandler))
			plan.edges = append(plan.edges, edgeSpec{
				key:        "gopls:caller:" + key,
				sourceKey:  key,
				targetKey:  plan.primaryNodeKey,
				kind:       EdgeCalls,
				status:     EdgeStatusObserved,
				label:      "calls changed code",
				confidence: 0.78,
				metadata: map[string]any{
					"source":      "gopls_call_hierarchy",
					"direction":   "incoming",
					"explanation": fmt.Sprintf("%s calls into the issue function, so it is a likely entry point for reproducing or understanding the finding.", trimOrDefault(entry.symbol, "This function")),
				},
			})
		case "callee":
			key := fmt.Sprintf("gopls:callee:%s:%d:%s", entry.path, entry.line, entry.symbol)
			plan.nodes = append(plan.nodes, goplsNodeSpec(key, entry, NodeRelatedCode))
			plan.edges = append(plan.edges, edgeSpec{
				key:        "gopls:callee:" + key,
				sourceKey:  plan.primaryNodeKey,
				targetKey:  key,
				kind:       EdgeCalls,
				status:     EdgeStatusObserved,
				label:      "calls",
				confidence: 0.72,
				metadata: map[string]any{
					"source":      "gopls_call_hierarchy",
					"direction":   "outgoing",
					"explanation": fmt.Sprintf("The issue function calls %s, which helps explain the downstream behavior involved in the finding.", trimOrDefault(entry.symbol, "this function")),
				},
			})
		}
	}
}

func callHierarchyTarget(repoRoot string, targetPath string, line int64) (int64, int) {
	symbol, ok := codeintel.ResolveEnclosingSymbol(repoRoot, targetPath, line)
	if !ok || symbol.NameLine <= 0 || symbol.NameColumn <= 0 {
		return line, 1
	}
	return symbol.NameLine, symbol.NameColumn
}

func annotatePrimarySymbol(primary *nodeSpec, symbol codeintel.Symbol) {
	if primary == nil || symbol.Name == "" {
		return
	}
	if primary.metadata == nil {
		primary.metadata = map[string]any{}
	}
	displayName := trimOrDefault(symbol.QualifiedName, symbol.Name)
	primary.symbol = displayName
	primary.metadata["symbol"] = displayName
	primary.metadata["enclosing_symbol"] = displayName
	primary.metadata["symbol_kind"] = symbol.Kind
	primary.metadata["symbol_language"] = symbol.Language
	primary.metadata["symbol_provenance"] = symbol.Provenance
	primary.metadata["symbol_start_line"] = symbol.StartLine
	primary.metadata["symbol_end_line"] = symbol.EndLine
}

func enrichMapPlanWithLocalCallers(plan *mapBuildPlan, repoRoot string, symbol codeintel.Symbol, fallbackReason string) bool {
	if plan == nil || symbol.Name == "" {
		return false
	}
	callers := codeintel.FindCallers(repoRoot, symbol, goplsCallHierarchyLimit)
	if len(callers) == 0 {
		return false
	}
	plan.callHierarchyAvailable = true
	source := localCallScanSource(symbol)
	plan.callHierarchySource = source
	reason := strings.TrimSpace(fallbackReason)
	if reason != "" {
		plan.callHierarchyReason = reason + "; " + callHierarchySourceLabel(source) + " added direct callers"
	} else {
		plan.callHierarchyReason = callHierarchySourceLabel(source) + " added direct callers"
	}
	if plan.callHierarchyTarget == "" {
		plan.callHierarchyTarget = fmt.Sprintf("%s:%d:%d", symbol.Path, symbol.NameLine, symbol.NameColumn)
	}
	for index, caller := range callers {
		key := fmt.Sprintf("%s:caller:%d:%s:%d", source, index, caller.Path, caller.Line)
		plan.nodes = append(plan.nodes, localCallSiteNodeSpec(key, caller, source))
		plan.edges = append(plan.edges, edgeSpec{
			key:        source + ":caller:" + key,
			sourceKey:  key,
			targetKey:  plan.primaryNodeKey,
			kind:       EdgeCalls,
			status:     EdgeStatusObserved,
			label:      "calls changed code",
			confidence: 0.68,
			metadata: map[string]any{
				"source":      source,
				"direction":   "incoming",
				"explanation": fmt.Sprintf("%s directly calls %s. This bundled fallback is syntax-based, so it is useful reachability evidence to inspect when an LSP call hierarchy is unavailable or incomplete.", trimOrDefault(caller.Caller.QualifiedName, "This function"), trimOrDefault(symbol.QualifiedName, symbol.Name)),
			},
		})
	}
	return true
}

func localCallSiteNodeSpec(key string, caller codeintel.CallSite, source string) nodeSpec {
	label := trimOrDefault(caller.Caller.QualifiedName, lineLabel(caller.Path, caller.Line, caller.Line))
	return nodeSpec{
		key:        key,
		kind:       NodeHandler,
		label:      label,
		path:       caller.Path,
		symbol:     caller.Caller.QualifiedName,
		startLine:  caller.Line,
		endLine:    caller.Line,
		confidence: 0.68,
		metadata: map[string]any{
			"source":            source,
			"direction":         "caller",
			"enclosing_symbol":  caller.Caller.QualifiedName,
			"symbol_language":   caller.Caller.Language,
			"symbol_start_line": caller.Caller.StartLine,
			"symbol_end_line":   caller.Caller.EndLine,
			"call_line":         caller.Line,
			"explanation":       "Bundled local code-intelligence found a direct call to the issue function. This baseline does not require system tree-sitter, gopls, or another language server.",
		},
	}
}

func localCallScanSource(symbol codeintel.Symbol) string {
	if symbol.Language == "go" || symbol.Provenance == "go_ast" {
		return "go_ast_call_scan"
	}
	return "heuristic_call_scan"
}

func mapPlanNode(plan *mapBuildPlan, key string) *nodeSpec {
	for index := range plan.nodes {
		if plan.nodes[index].key == key {
			return &plan.nodes[index]
		}
	}
	return nil
}

func goplsNodeSpec(key string, entry goplsHierarchyEntry, kind string) nodeSpec {
	direction := "related function"
	if entry.direction == "caller" {
		direction = "caller"
	} else if entry.direction == "callee" {
		direction = "callee"
	}
	return nodeSpec{
		key:        key,
		kind:       kind,
		label:      trimOrDefault(entry.symbol, lineLabel(entry.path, entry.line, entry.line)),
		path:       entry.path,
		symbol:     entry.symbol,
		startLine:  entry.line,
		endLine:    entry.line,
		confidence: 0.75,
		metadata: map[string]any{
			"source":      "gopls_call_hierarchy",
			"direction":   entry.direction,
			"explanation": fmt.Sprintf("gopls identified this %s of the issue function, showing how the finding connects to nearby code.", direction),
		},
	}
}

func selectGoplsHierarchyEntries(entries []goplsHierarchyEntry, limit int) []goplsHierarchyEntry {
	if len(entries) == 0 || limit == 0 {
		return nil
	}
	deduped := make([]goplsHierarchyEntry, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.direction != "caller" && entry.direction != "callee" {
			continue
		}
		key := entry.direction + "|" + entry.path + "|" + strconv.FormatInt(entry.line, 10) + "|" + entry.symbol
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, entry)
	}
	sort.SliceStable(deduped, func(i, j int) bool {
		leftRank := goplsHierarchyEntryRank(deduped[i])
		rightRank := goplsHierarchyEntryRank(deduped[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftTest := strings.HasSuffix(deduped[i].path, "_test.go")
		rightTest := strings.HasSuffix(deduped[j].path, "_test.go")
		if leftTest != rightTest {
			return !leftTest
		}
		if deduped[i].path != deduped[j].path {
			return deduped[i].path < deduped[j].path
		}
		if deduped[i].line != deduped[j].line {
			return deduped[i].line < deduped[j].line
		}
		return deduped[i].symbol < deduped[j].symbol
	})
	if limit > 0 && len(deduped) > limit {
		return keepGoplsDirectionCoverage(deduped, limit)
	}
	return deduped
}

func keepGoplsDirectionCoverage(entries []goplsHierarchyEntry, limit int) []goplsHierarchyEntry {
	if limit <= 0 || len(entries) <= limit {
		return entries
	}
	selected := make([]goplsHierarchyEntry, 0, limit)
	selectedKeys := map[string]struct{}{}
	add := func(entry goplsHierarchyEntry) {
		if len(selected) >= limit {
			return
		}
		key := entry.direction + "|" + entry.path + "|" + strconv.FormatInt(entry.line, 10) + "|" + entry.symbol
		if _, ok := selectedKeys[key]; ok {
			return
		}
		selectedKeys[key] = struct{}{}
		selected = append(selected, entry)
	}
	if limit >= 2 {
		for _, entry := range entries {
			if entry.direction == "caller" {
				add(entry)
				break
			}
		}
		for _, entry := range entries {
			if entry.direction == "callee" {
				add(entry)
				break
			}
		}
	}
	for _, entry := range entries {
		add(entry)
	}
	return selected
}

func goplsHierarchyEntryRank(entry goplsHierarchyEntry) int {
	switch entry.direction {
	case "caller":
		return 0
	case "callee":
		return 1
	default:
		return 9
	}
}

func parseGoplsCallHierarchy(output string, repoRoot string) (*goplsHierarchyEntry, []goplsHierarchyEntry) {
	var identifier *goplsHierarchyEntry
	var entries []goplsHierarchyEntry
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if match := goplsHierarchyIdentifierRE.FindStringSubmatch(line); len(match) == 4 {
			if entry, ok := newGoplsHierarchyEntry("identifier", match[1], match[2], match[3], repoRoot); ok {
				identifier = &entry
			}
			continue
		}
		if match := goplsHierarchyRelationRE.FindStringSubmatch(line); len(match) == 5 {
			if entry, ok := newGoplsHierarchyEntry(match[1], match[2], match[3], match[4], repoRoot); ok {
				entries = append(entries, entry)
			}
		}
	}
	return identifier, entries
}

func newGoplsHierarchyEntry(direction string, symbol string, path string, lineText string, repoRoot string) (goplsHierarchyEntry, bool) {
	line, err := strconv.ParseInt(lineText, 10, 64)
	if err != nil || line <= 0 {
		return goplsHierarchyEntry{}, false
	}
	normalizedPath, ok := normalizeGoplsPath(path, repoRoot)
	if !ok {
		return goplsHierarchyEntry{}, false
	}
	return goplsHierarchyEntry{
		direction: direction,
		symbol:    strings.TrimSpace(symbol),
		path:      normalizedPath,
		line:      line,
	}, true
}

func normalizeGoplsPath(path string, repoRoot string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	if filepath.IsAbs(path) && repoRoot != "" {
		if rel, err := filepath.Rel(repoRoot, path); err == nil && !isParentRelativePath(rel) {
			return filepath.ToSlash(rel), true
		}
		return "", false
	}
	normalized := filepath.ToSlash(path)
	if normalized == "." || strings.HasPrefix(normalized, "../") {
		return "", false
	}
	return normalized, true
}

func findGoModuleRoot(repoRoot string, targetPath string) (string, bool) {
	repoRoot = filepath.Clean(repoRoot)
	targetAbs := filepath.Join(repoRoot, filepath.FromSlash(targetPath))
	if !strings.HasSuffix(filepath.Base(targetAbs), ".go") {
		return "", false
	}
	dir := filepath.Dir(targetAbs)
	for {
		if rel, err := filepath.Rel(repoRoot, dir); err != nil || isParentRelativePath(rel) {
			return "", false
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		if dir == repoRoot {
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func isParentRelativePath(path string) bool {
	path = filepath.Clean(path)
	return path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func primaryNodeSpec(finding dbgen.Finding, primaryEvidence *dbgen.EvidenceItem) nodeSpec {
	path := nullableStringValue(finding.PrimaryPath)
	startLine := nullableInt64Value(finding.PrimaryStartLine)
	endLine := nullableInt64Value(finding.PrimaryEndLine)
	if primaryEvidence != nil {
		if path == "" {
			path = nullableStringValue(primaryEvidence.Path)
		}
		if startLine <= 0 {
			startLine = nullableInt64Value(primaryEvidence.StartLine)
		}
		if endLine <= 0 {
			endLine = nullableInt64Value(primaryEvidence.EndLine)
		}
	}
	if endLine == 0 {
		endLine = startLine
	}
	label := "Primary changed code"
	if path != "" {
		if startLine > 0 {
			label = lineLabel(path, startLine, endLine)
		} else {
			label = "Changed file needs a line anchor"
		}
	}
	spec := nodeSpec{
		key:        "primary",
		kind:       NodeChangedCode,
		label:      label,
		path:       path,
		startLine:  startLine,
		endLine:    endLine,
		confidence: clampConfidence(finding.Confidence),
		metadata: map[string]any{
			"source":      "finding_location",
			"role":        "primary",
			"explanation": "This is the exact file and line range the finding is anchored to; other nodes should explain reachability, support, tests, or real contradictions around this location.",
		},
	}
	if primaryEvidence != nil {
		spec.evidenceItemID = primaryEvidence.ID
		spec.confidence = clampConfidence(primaryEvidence.Confidence)
		spec.metadata["evidence_item_id"] = primaryEvidence.ID
	}
	if path != "" && startLine <= 0 {
		spec.kind = NodeUnknown
		spec.metadata["reason"] = "missing_primary_line"
	}
	return spec
}

func evidenceNodeSpec(finding dbgen.Finding, item dbgen.EvidenceItem) nodeSpec {
	path := nullableStringValue(item.Path)
	startLine := nullableInt64Value(item.StartLine)
	endLine := nullableInt64Value(item.EndLine)
	if endLine == 0 {
		endLine = startLine
	}
	kind := nodeKindForEvidence(item)
	label := strings.TrimSpace(item.Title)
	if label == "" && path != "" {
		label = lineLabel(path, startLine, endLine)
	}
	if label == "" {
		label = "Evidence item"
	}
	metadata := map[string]any{
		"source":           "evidence_item",
		"evidence_kind":    item.Kind,
		"evidence_item_id": item.ID,
		"explanation":      evidenceItemExplanation(item),
	}
	for _, key := range []string{"producer", "rule", "source"} {
		if value := metadataString(item.MetadataJson, key); value != "" {
			metadata["evidence_"+key] = value
		}
	}
	if path != "" {
		metadata["deep_link"] = map[string]any{
			"kind":       "file",
			"path":       path,
			"start_line": startLine,
			"end_line":   endLine,
		}
	}
	if kind == NodeMissingGuard {
		profile := classifyRuleProfile(finding)
		metadata["rule"] = profile.ID
	}
	return nodeSpec{
		key:            "evidence:" + item.ID,
		kind:           kind,
		label:          label,
		path:           path,
		startLine:      startLine,
		endLine:        endLine,
		evidenceItemID: item.ID,
		confidence:     clampConfidence(item.Confidence),
		metadata:       metadata,
	}
}

func evidenceEdgeSpec(item dbgen.EvidenceItem, primaryKey string, nodeKey string) edgeSpec {
	switch item.Kind {
	case KindCounter:
		return edgeSpec{
			key:        "counter:" + item.ID,
			sourceKey:  nodeKey,
			targetKey:  primaryKey,
			kind:       EdgeContradicts,
			status:     EdgeStatusObserved,
			label:      "verified contradiction",
			confidence: clampConfidence(item.Confidence),
			metadata: map[string]any{
				"source":           "evidence_item",
				"evidence_item_id": item.ID,
				"explanation":      "This is treated as counter-evidence only because it claims to directly refute the finding, not merely because it matched a keyword nearby.",
			},
		}
	case KindTest:
		return edgeSpec{
			key:        "test:" + item.ID,
			sourceKey:  nodeKey,
			targetKey:  primaryKey,
			kind:       EdgeTests,
			status:     EdgeStatusObserved,
			label:      "test coverage signal",
			confidence: clampConfidence(item.Confidence),
			metadata: map[string]any{
				"source":           "evidence_item",
				"evidence_item_id": item.ID,
				"explanation":      "This test signal helps you check coverage or expected behavior; it does not automatically refute the finding.",
			},
		}
	case KindMissing:
		kind := EdgeSupports
		status := EdgeStatusObserved
		label := "missing evidence"
		if nodeKindForEvidence(item) == NodeMissingGuard {
			kind = EdgeMissingGuard
			status = EdgeStatusMissing
			label = "missing expected guard"
		}
		return edgeSpec{
			key:        "missing:" + item.ID,
			sourceKey:  primaryKey,
			targetKey:  nodeKey,
			kind:       kind,
			status:     status,
			label:      label,
			confidence: clampConfidence(item.Confidence),
			metadata: map[string]any{
				"source":           "evidence_item",
				"evidence_item_id": item.ID,
				"explanation":      "This records a missing proof or missing relationship that still needs verification against the code path.",
			},
		}
	case KindSearch:
		return edgeSpec{
			key:        "lead:" + item.ID,
			sourceKey:  nodeKey,
			targetKey:  primaryKey,
			kind:       EdgeSupports,
			status:     EdgeStatusObserved,
			label:      "verification lead",
			confidence: clampConfidence(item.Confidence),
			metadata: map[string]any{
				"source":           "evidence_item",
				"evidence_item_id": item.ID,
				"explanation":      "This is a related lead to inspect. It is intentionally not labeled counter-evidence unless it directly disproves the claim.",
			},
		}
	case KindStaticAnalysis:
		relationship := strings.ToLower(strings.TrimSpace(metadataString(item.MetadataJson, "relationship")))
		sourceKey := nodeKey
		targetKey := primaryKey
		label := "code relationship"
		if relationship == "callee" || relationship == "downstream" {
			sourceKey = primaryKey
			targetKey = nodeKey
			label = relationship
		} else if relationship == "caller" || relationship == "entrypoint" {
			label = relationship
		}
		return edgeSpec{
			key:        "relationship:" + item.ID,
			sourceKey:  sourceKey,
			targetKey:  targetKey,
			kind:       EdgeSupports,
			status:     EdgeStatusObserved,
			label:      label,
			confidence: clampConfidence(item.Confidence),
			metadata: map[string]any{
				"source":           "evidence_item",
				"evidence_item_id": item.ID,
				"relationship":     relationship,
				"explanation":      "This relationship evidence explains how the issue location connects to callers, callees, entry points, or downstream behavior.",
			},
		}
	case KindSupporting, KindAgent, KindNeutral:
		return edgeSpec{
			key:        "support:" + item.ID,
			sourceKey:  nodeKey,
			targetKey:  primaryKey,
			kind:       EdgeSupports,
			status:     EdgeStatusObserved,
			label:      "supports finding",
			confidence: clampConfidence(item.Confidence),
			metadata: map[string]any{
				"source":           "evidence_item",
				"evidence_item_id": item.ID,
				"explanation":      "This evidence supports or contextualizes the primary issue location.",
			},
		}
	default:
		return edgeSpec{}
	}
}

func syntheticMissingRelationshipNode(finding dbgen.Finding, items []dbgen.EvidenceItem) nodeSpec {
	if hasMissingGuardNode(items) {
		return nodeSpec{}
	}
	text := strings.ToLower(strings.Join([]string{
		finding.CanonicalClaim,
		finding.Category,
		nullableStringValue(finding.SuggestedFix),
		nullableStringValue(finding.DraftComment),
	}, " "))
	if !containsAny(text, "missing", "miss ", "misses", "lacks", "without", "does not", "not enforce", "skipped", "bypass") {
		return nodeSpec{}
	}
	profile := classifyRuleProfile(finding)
	if profile.ID != "auth_guard" && profile.ID != "webhook_validation" {
		return nodeSpec{}
	}
	label := "Expected guard is missing"
	if profile.ID == "webhook_validation" {
		label = "Expected webhook validation is missing"
	}
	return nodeSpec{
		key:        "missing:" + profile.ID,
		kind:       NodeMissingGuard,
		label:      label,
		confidence: clampConfidence(finding.Confidence),
		metadata: map[string]any{
			"source":      "local_rule",
			"rule":        profile.ID,
			"explanation": "The finding language implies an expected guard or validation relationship, but no verified relationship has been attached yet.",
		},
	}
}

func callPathSteps(nodes []nodeSpec, edges []edgeSpec, primaryKey string) ([]callStepSpec, string) {
	byKey := make(map[string]nodeSpec, len(nodes))
	for _, node := range nodes {
		byKey[node.key] = node
	}
	primary, ok := byKey[primaryKey]
	if !ok || primary.path == "" {
		return nil, "Call path unavailable because the finding has no primary code location."
	}
	steps := []callStepSpec{callStepFromNode(primary)}
	for _, kind := range []string{EdgeCalls, EdgeMounts, EdgeMissingGuard, EdgeProtects, EdgeTests, EdgeSupports, EdgeContradicts} {
		for _, edge := range edges {
			if len(steps) >= defaultCallPathStepLimit {
				break
			}
			if edge.kind != kind {
				continue
			}
			key := edge.targetKey
			if key == primaryKey {
				key = edge.sourceKey
			}
			if key == primaryKey || containsCallStep(steps, key) {
				continue
			}
			node, ok := byKey[key]
			if !ok {
				continue
			}
			steps = append(steps, callStepFromNode(node))
		}
	}
	if len(steps) == 1 {
		return steps, "Only the primary location is available; no related evidence path could be inferred."
	}
	return steps, ""
}

func callStepFromNode(node nodeSpec) callStepSpec {
	return callStepSpec{
		nodeKey:   node.key,
		path:      node.path,
		startLine: node.startLine,
		endLine:   node.endLine,
		label:     node.label,
	}
}

func containsCallStep(steps []callStepSpec, nodeKey string) bool {
	for _, step := range steps {
		if step.nodeKey == nodeKey {
			return true
		}
	}
	return false
}

func (s *Service) mapView(ctx context.Context, finding dbgen.Finding, graph dbgen.EvidenceGraph) (MapView, error) {
	repoRoot := s.mapRepositoryRoot(ctx, finding)
	nodes, err := s.Queries.ListEvidenceNodesByGraph(ctx, graph.ID)
	if err != nil {
		return MapView{}, fmt.Errorf("list evidence map nodes: %w", err)
	}
	edges, err := s.Queries.ListEvidenceEdgesByGraph(ctx, graph.ID)
	if err != nil {
		return MapView{}, fmt.Errorf("list evidence map edges: %w", err)
	}
	paths, err := s.Queries.ListCallPathsByGraph(ctx, graph.ID)
	if err != nil {
		return MapView{}, fmt.Errorf("list evidence map call paths: %w", err)
	}
	items, err := s.Queries.ListEvidenceItemsByFinding(ctx, finding.ID)
	if err != nil {
		return MapView{}, fmt.Errorf("list evidence map panel evidence: %w", err)
	}
	layout := json.RawMessage(graph.LayoutJson)
	if len(layout) == 0 || !json.Valid(layout) {
		layout = json.RawMessage("{}")
	}
	nodeViews := make([]NodeView, 0, len(nodes))
	visibleNodeIDs := map[string]struct{}{}
	for _, node := range nodes {
		view := nodeView(node, repoRoot)
		if !shouldDisplayEvidenceMapNode(view) {
			continue
		}
		nodeViews = append(nodeViews, view)
		visibleNodeIDs[view.ID] = struct{}{}
	}
	edgeViews := make([]EdgeView, 0, len(edges))
	for _, edge := range edges {
		view := edgeView(edge)
		if _, ok := visibleNodeIDs[view.Source]; !ok {
			continue
		}
		if _, ok := visibleNodeIDs[view.Target]; !ok {
			continue
		}
		edgeViews = append(edgeViews, view)
	}
	callPathViews := make([]CallPathView, 0, len(paths))
	for _, path := range paths {
		steps, err := s.Queries.ListCallPathStepsByCallPath(ctx, path.ID)
		if err != nil {
			return MapView{}, fmt.Errorf("list evidence map call path steps: %w", err)
		}
		view := callPathView(path, steps)
		view.Steps = visibleCallPathSteps(view.Steps, visibleNodeIDs)
		if len(view.Steps) > 0 {
			callPathViews = append(callPathViews, view)
		}
	}
	var primaryCallPath []CallPathStepView
	if len(callPathViews) > 0 {
		primaryCallPath = callPathViews[0].Steps
	}
	missingReasons, callPathReason, connectionSummary := layoutNarrative(layout)
	return MapView{
		Graph: GraphView{
			ID:              graph.ID,
			FindingID:       graph.FindingID,
			ReviewSessionID: graph.ReviewSessionID,
			Status:          graph.Status,
			Summary:         nullableStringValue(graph.Summary),
			Layout:          layout,
			CreatedAt:       graph.CreatedAt,
			UpdatedAt:       graph.UpdatedAt,
		},
		Finding:                   mapFindingView(finding),
		Hierarchy:                 hierarchyItems(nodeViews),
		Nodes:                     nodeViews,
		Edges:                     edgeViews,
		CallPath:                  primaryCallPath,
		CallPaths:                 callPathViews,
		CallPathUnavailableReason: callPathReason,
		Legend:                    defaultLegend(),
		Panel: MapPanel{
			Claim:                  finding.CanonicalClaim,
			Severity:               finding.Severity,
			VerificationStatus:     finding.VerificationStatus,
			DecisionStatus:         finding.DecisionStatus,
			ConnectionSummary:      connectionSummary,
			EvidenceSummary:        nullableStringValue(finding.EvidenceSummary),
			CounterEvidenceSummary: nullableStringValue(finding.CounterEvidenceSummary),
			SuggestedFix:           nullableStringValue(finding.SuggestedFix),
			EvidenceCounts:         evidenceItemKindCounts(visibleEvidenceItems(items)),
			Evidence:               panelEvidenceItems(visibleEvidenceItems(items), repoRoot),
		},
		MissingReasons: missingReasons,
	}, nil
}

func (s *Service) mapRepositoryRoot(ctx context.Context, finding dbgen.Finding) string {
	if s == nil || s.Queries == nil {
		return ""
	}
	session, err := s.Queries.GetReviewSession(ctx, finding.ReviewSessionID)
	if err != nil || strings.TrimSpace(session.RepositoryID) == "" {
		return ""
	}
	repository, err := s.Queries.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(repository.LocalPath)
}

func nodeView(row dbgen.EvidenceNode, repoRoot string) NodeView {
	metadata := json.RawMessage(row.MetadataJson)
	if len(metadata) == 0 || !json.Valid(metadata) {
		metadata = json.RawMessage("{}")
	}
	path := nullableStringValue(row.Path)
	startLine := nullableInt64Value(row.StartLine)
	endLine := nullableInt64Value(row.EndLine)
	var deepLink *NodeDeepLink
	if path != "" {
		deepLink = &NodeDeepLink{
			Kind:      "file",
			Path:      path,
			StartLine: startLine,
			EndLine:   endLine,
		}
	}
	codeSnippet, lineWindow := sourcePreviewForLocation(repoRoot, path, startLine, endLine)
	if strings.TrimSpace(codeSnippet) == "" {
		codeSnippet, lineWindow = sourcePreviewFromMetadata(metadata, startLine, endLine)
	}
	fileContent, fileLineCount, fileTruncated := sourceFileForLocation(repoRoot, path)
	return NodeView{
		ID:             row.ID,
		Kind:           row.Kind,
		Label:          row.Label,
		Explanation:    metadataJSONString(metadata, "explanation"),
		Path:           path,
		Symbol:         nullableStringValue(row.Symbol),
		StartLine:      startLine,
		EndLine:        endLine,
		EvidenceItemID: nullableStringValue(row.EvidenceItemID),
		Confidence:     row.Confidence,
		DeepLink:       deepLink,
		CodeSnippet:    codeSnippet,
		LineWindow:     lineWindow,
		FileContent:    fileContent,
		FileLineCount:  fileLineCount,
		FileTruncated:  fileTruncated,
		Metadata:       metadata,
	}
}

func edgeView(row dbgen.EvidenceEdge) EdgeView {
	metadata := json.RawMessage(row.MetadataJson)
	if len(metadata) == 0 || !json.Valid(metadata) {
		metadata = json.RawMessage("{}")
	}
	return EdgeView{
		ID:          row.ID,
		Source:      row.SourceNodeID,
		Target:      row.TargetNodeID,
		Kind:        row.Kind,
		Status:      row.Status,
		Label:       nullableStringValue(row.Label),
		Explanation: metadataJSONString(metadata, "explanation"),
		Confidence:  row.Confidence,
		Metadata:    metadata,
	}
}

func callPathView(row dbgen.CallPath, steps []dbgen.CallPathStep) CallPathView {
	view := CallPathView{
		ID:         row.ID,
		Label:      nullableStringValue(row.Label),
		Summary:    callPathSummary(steps),
		Confidence: row.Confidence,
		Steps:      make([]CallPathStepView, 0, len(steps)),
	}
	for _, step := range steps {
		view.Steps = append(view.Steps, CallPathStepView{
			ID:          step.ID,
			NodeID:      nullableStringValue(step.NodeID),
			StepIndex:   step.StepIndex,
			Path:        nullableStringValue(step.Path),
			StartLine:   nullableInt64Value(step.StartLine),
			EndLine:     nullableInt64Value(step.EndLine),
			Label:       step.Label,
			Explanation: callPathStepExplanation(step),
		})
	}
	return view
}

func mapFindingView(row dbgen.Finding) MapFindingView {
	return MapFindingView{
		ID:                     row.ID,
		ReviewSessionID:        row.ReviewSessionID,
		CanonicalClaim:         row.CanonicalClaim,
		Category:               row.Category,
		Severity:               row.Severity,
		Confidence:             row.Confidence,
		VerificationStatus:     row.VerificationStatus,
		DecisionStatus:         row.DecisionStatus,
		PrimaryPath:            nullableStringValue(row.PrimaryPath),
		PrimaryStartLine:       nullableInt64Value(row.PrimaryStartLine),
		PrimaryEndLine:         nullableInt64Value(row.PrimaryEndLine),
		EvidenceSummary:        nullableStringValue(row.EvidenceSummary),
		CounterEvidenceSummary: nullableStringValue(row.CounterEvidenceSummary),
		SuggestedFix:           nullableStringValue(row.SuggestedFix),
	}
}

func hierarchyItems(nodes []NodeView) []HierarchyItem {
	byPath := map[string]*HierarchyItem{}
	for _, node := range nodes {
		if node.Path == "" {
			continue
		}
		item := byPath[node.Path]
		if item == nil {
			item = &HierarchyItem{
				Path:      node.Path,
				Kind:      node.Kind,
				StartLine: node.StartLine,
				EndLine:   node.EndLine,
			}
			byPath[node.Path] = item
		}
		item.NodeIDs = append(item.NodeIDs, node.ID)
		if node.EvidenceItemID != "" {
			item.EvidenceItemIDs = append(item.EvidenceItemIDs, node.EvidenceItemID)
		}
		if item.StartLine == 0 || (node.StartLine > 0 && node.StartLine < item.StartLine) {
			item.StartLine = node.StartLine
		}
		if node.EndLine > item.EndLine {
			item.EndLine = node.EndLine
		}
	}
	items := make([]HierarchyItem, 0, len(byPath))
	for _, item := range byPath {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	return items
}

func panelEvidenceItems(items []dbgen.EvidenceItem, repoRoot string) []MapPanelEvidenceRef {
	result := make([]MapPanelEvidenceRef, 0, len(items))
	for _, item := range items {
		metadata := json.RawMessage(item.MetadataJson)
		if len(metadata) == 0 || !json.Valid(metadata) {
			metadata = json.RawMessage("{}")
		}
		path := nullableStringValue(item.Path)
		startLine := nullableInt64Value(item.StartLine)
		endLine := nullableInt64Value(item.EndLine)
		codeSnippet, lineWindow := sourcePreviewForLocation(repoRoot, path, startLine, endLine)
		if strings.TrimSpace(codeSnippet) == "" {
			codeSnippet, lineWindow = sourcePreviewFromMetadata(metadata, startLine, endLine)
		}
		fileContent, fileLineCount, fileTruncated := sourceFileForLocation(repoRoot, path)
		result = append(result, MapPanelEvidenceRef{
			ID:            item.ID,
			Kind:          item.Kind,
			Title:         item.Title,
			Summary:       item.Summary,
			Path:          path,
			StartLine:     startLine,
			EndLine:       endLine,
			Confidence:    item.Confidence,
			CodeSnippet:   codeSnippet,
			LineWindow:    lineWindow,
			FileContent:   fileContent,
			FileLineCount: fileLineCount,
			FileTruncated: fileTruncated,
		})
	}
	return result
}

func sourcePreviewForLocation(repoRoot string, path string, startLine int64, endLine int64) (string, *SourceWindow) {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(path) == "" || startLine <= 0 {
		return "", nil
	}
	if endLine < startLine {
		endLine = startLine
	}
	snippet, windowStart, windowEnd, _, err := ReadSnippet(
		repoRoot,
		path,
		startLine,
		endLine,
		mapSourceContextLines,
		mapSourceMaxSnippetBytes,
	)
	if err != nil || strings.TrimSpace(snippet) == "" {
		return "", nil
	}
	return snippet, &SourceWindow{StartLine: windowStart, EndLine: windowEnd}
}

func sourceFileForLocation(repoRoot string, path string) (string, int64, bool) {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(path) == "" {
		return "", 0, false
	}
	content, lineCount, truncated, err := ReadSourceFile(repoRoot, path, mapSourceMaxFileBytes)
	if err != nil || strings.TrimSpace(content) == "" {
		return "", 0, false
	}
	return content, lineCount, truncated
}

func sourcePreviewFromMetadata(metadata json.RawMessage, startLine int64, endLine int64) (string, *SourceWindow) {
	if len(metadata) == 0 || !json.Valid(metadata) {
		return "", nil
	}
	var payload struct {
		CodeSnippet   string          `json:"code_snippet"`
		AgentMetadata json.RawMessage `json:"agent_metadata"`
		LineWindow    struct {
			StartLine int64 `json:"start_line"`
			EndLine   int64 `json:"end_line"`
		} `json:"line_window"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return "", nil
	}
	snippet := strings.TrimSpace(payload.CodeSnippet)
	if snippet == "" {
		return sourcePreviewFromMetadata(payload.AgentMetadata, startLine, endLine)
	}
	var window *SourceWindow
	if payload.LineWindow.StartLine > 0 && payload.LineWindow.EndLine >= payload.LineWindow.StartLine {
		window = &SourceWindow{
			StartLine: payload.LineWindow.StartLine,
			EndLine:   payload.LineWindow.EndLine,
		}
	} else if startLine > 0 {
		windowEnd := endLine
		if windowEnd < startLine {
			windowEnd = startLine + int64(strings.Count(snippet, "\n"))
		}
		window = &SourceWindow{StartLine: startLine, EndLine: windowEnd}
	}
	return snippet, window
}

func boundedEvidenceItemsForGraph(items []dbgen.EvidenceItem, primary *dbgen.EvidenceItem) ([]dbgen.EvidenceItem, int) {
	filtered := make([]dbgen.EvidenceItem, 0, len(items))
	for _, item := range items {
		if primary != nil && item.ID == primary.ID {
			continue
		}
		if !shouldUseEvidenceItemInMap(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		leftRank := evidenceMapKindRank(filtered[i].Kind)
		rightRank := evidenceMapKindRank(filtered[j].Kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if filtered[i].Confidence != filtered[j].Confidence {
			return filtered[i].Confidence > filtered[j].Confidence
		}
		if filtered[i].CreatedAt != filtered[j].CreatedAt {
			return filtered[i].CreatedAt < filtered[j].CreatedAt
		}
		return filtered[i].ID < filtered[j].ID
	})
	if len(filtered) <= defaultEvidenceMapItemLimit {
		return filtered, 0
	}
	return filtered[:defaultEvidenceMapItemLimit], len(filtered) - defaultEvidenceMapItemLimit
}

func visibleEvidenceItems(items []dbgen.EvidenceItem) []dbgen.EvidenceItem {
	filtered := make([]dbgen.EvidenceItem, 0, len(items))
	for _, item := range items {
		if !shouldUseEvidenceItemInMap(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func shouldUseEvidenceItemInMap(item dbgen.EvidenceItem) bool {
	path := nullableStringValue(item.Path)
	if path == "" {
		return true
	}
	if !isProjectMetadataPath(path) {
		return true
	}
	return item.Kind == KindSupporting && metadataString(item.MetadataJson, "source") == "primary_location"
}

func shouldDisplayEvidenceMapNode(node NodeView) bool {
	if node.Path == "" || !isProjectMetadataPath(node.Path) {
		return true
	}
	return node.Kind == NodeChangedCode
}

func visibleCallPathSteps(steps []CallPathStepView, visibleNodeIDs map[string]struct{}) []CallPathStepView {
	filtered := make([]CallPathStepView, 0, len(steps))
	for _, step := range steps {
		if step.NodeID == "" {
			filtered = append(filtered, step)
			continue
		}
		if _, ok := visibleNodeIDs[step.NodeID]; ok {
			filtered = append(filtered, step)
		}
	}
	return filtered
}

func evidenceMapKindRank(kind string) int {
	switch kind {
	case KindSupporting:
		return 0
	case KindStaticAnalysis:
		return 1
	case KindCounter:
		return 2
	case KindTest:
		return 3
	case KindMissing:
		return 4
	case KindSearch:
		return 5
	case KindAgent:
		return 6
	default:
		return 7
	}
}

func primaryEvidenceItem(finding dbgen.Finding, items []dbgen.EvidenceItem) *dbgen.EvidenceItem {
	findingPath := cleanPathKey(nullableStringValue(finding.PrimaryPath))
	startLine := nullableInt64Value(finding.PrimaryStartLine)
	for index := range items {
		item := &items[index]
		if item.Kind != KindSupporting || metadataString(item.MetadataJson, "source") != "primary_location" {
			continue
		}
		if findingPath != "" && cleanPathKey(nullableStringValue(item.Path)) != findingPath {
			continue
		}
		if startLine > 0 && nullableInt64Value(item.StartLine) > 0 && nullableInt64Value(item.StartLine) != startLine {
			continue
		}
		return item
	}
	return nil
}

func nodeKindForEvidence(item dbgen.EvidenceItem) string {
	path := strings.ToLower(filepath.ToSlash(nullableStringValue(item.Path)))
	text := strings.ToLower(strings.Join([]string{item.Title, item.Summary, path}, " "))
	relationship := strings.ToLower(strings.TrimSpace(metadataString(item.MetadataJson, "relationship")))
	switch item.Kind {
	case KindCounter:
		return NodeCounterEvidence
	case KindTest:
		return NodeTest
	case KindStaticAnalysis:
		switch relationship {
		case "entrypoint":
			return NodeEntrypoint
		case "caller":
			return NodeHandler
		case "callee", "downstream":
			return NodeRelatedCode
		}
	case KindMissing:
		if containsAny(text, "guard", "auth", "permission", "middleware", "signature", "hmac", "webhook") {
			return NodeMissingGuard
		}
		return NodeUnknown
	}
	switch {
	case isLikelyTestPath(path):
		return NodeTest
	case containsAny(path, "middleware") || containsAny(text, "middleware"):
		return NodeMiddleware
	case containsAny(path, "config", ".yaml", ".yml", ".toml", ".env") || containsAny(text, "config"):
		return NodeConfig
	case containsAny(path, "route", "routes") || containsAny(text, "route"):
		return NodeRoute
	case containsAny(path, "handler", "controller") || containsAny(text, "handler"):
		return NodeHandler
	case containsAny(path, "auth", "guard", "permission") || containsAny(text, "guard", "permission", "authorize"):
		return NodeGuard
	default:
		return NodeRelatedCode
	}
}

func hasMissingGuardNode(items []dbgen.EvidenceItem) bool {
	for _, item := range items {
		if nodeKindForEvidence(item) == NodeMissingGuard {
			return true
		}
	}
	return false
}

func mapSummaryText(finding dbgen.Finding, plan mapBuildPlan) string {
	claim := strings.TrimSpace(finding.CanonicalClaim)
	if claim == "" {
		claim = "finding"
	}
	if plan.status == GraphStatusPartial {
		return fmt.Sprintf("Partial evidence map for %q with %d node(s), %d edge(s), and %d missing reason(s).", claim, len(plan.nodes), len(plan.edges), len(plan.missingReasons))
	}
	return fmt.Sprintf("Evidence map for %q with %d node(s), %d edge(s), and %d call path step(s).", claim, len(plan.nodes), len(plan.edges), len(plan.callSteps))
}

func evidenceMapConnectionSummary(finding dbgen.Finding, plan mapBuildPlan) string {
	primary := mapPlanNode(&plan, plan.primaryNodeKey)
	issue := "the issue line"
	if primary != nil && primary.path != "" {
		issue = lineLabel(primary.path, primary.startLine, primary.endLine)
	}
	callSteps := 0
	for _, edge := range plan.edges {
		if edge.kind == EdgeCalls {
			callSteps++
		}
	}
	checks := 0
	contradictions := 0
	tests := 0
	for _, node := range plan.nodes {
		switch node.kind {
		case NodeCounterEvidence:
			contradictions++
			checks++
		case NodeTest:
			tests++
			checks++
		case NodeEntrypoint, NodeRoute, NodeGuard, NodeMiddleware, NodeConfig, NodeRelatedCode, NodeMissingGuard:
			if node.key != plan.primaryNodeKey {
				checks++
			}
		}
	}
	if callSteps > 0 {
		return fmt.Sprintf("The map anchors the claim at %s, then uses %s to show %d caller/callee relationship(s) and %d verification check(s).", issue, callHierarchySourceLabel(plan.callHierarchySource), callSteps, checks)
	}
	if contradictions > 0 {
		return fmt.Sprintf("The map anchors the claim at %s and highlights %d verified contradiction(s) to compare before accepting the finding.", issue, contradictions)
	}
	if tests > 0 || checks > 0 {
		return fmt.Sprintf("The map anchors the claim at %s and separates %d related check(s) from true counter-evidence so they can be inspected without overstating them.", issue, checks)
	}
	claim := strings.TrimSpace(finding.CanonicalClaim)
	if claim == "" {
		claim = "this finding"
	}
	return fmt.Sprintf("The map is currently anchored at %s for %q, but no connected call hierarchy or related check has been verified yet.", issue, claim)
}

func evidenceItemExplanation(item dbgen.EvidenceItem) string {
	switch item.Kind {
	case KindSupporting:
		return "This item supports the finding at the primary issue location."
	case KindCounter:
		return "This item is a verified contradiction only if it directly disproves reachability, behavior, or impact of the claim."
	case KindTest:
		return "This is a related test signal to inspect for expected behavior or missing coverage."
	case KindSearch:
		return "This is a verification lead from repository search, not counter-evidence by itself."
	case KindMissing:
		return "This records an absent proof or missing relationship that still needs code-path verification."
	case KindStaticAnalysis:
		return "This static-analysis signal adds context around the issue but should be checked against the exact code path."
	case KindAgent:
		return "This is reviewer-agent context attached to the finding."
	default:
		return "This item adds context to the evidence map."
	}
}

func evidenceItemKindCounts(items []dbgen.EvidenceItem) map[string]int {
	counts := map[string]int{
		KindSupporting:     0,
		KindCounter:        0,
		KindNeutral:        0,
		KindMissing:        0,
		KindTest:           0,
		KindSearch:         0,
		KindAgent:          0,
		KindStaticAnalysis: 0,
	}
	for _, item := range items {
		if _, ok := counts[item.Kind]; ok {
			counts[item.Kind]++
			continue
		}
		counts[KindNeutral]++
	}
	return counts
}

func callPathSummary(steps []dbgen.CallPathStep) string {
	if len(steps) == 0 {
		return ""
	}
	if len(steps) == 1 {
		return fmt.Sprintf("Only %s is currently available in the call path.", callPathStepShortLabel(steps[0]))
	}
	return fmt.Sprintf("%s connects to %s across %d step(s).", callPathStepShortLabel(steps[0]), callPathStepShortLabel(steps[len(steps)-1]), len(steps))
}

func callPathStepExplanation(step dbgen.CallPathStep) string {
	label := callPathStepShortLabel(step)
	if step.StepIndex == 0 {
		return fmt.Sprintf("%s is the issue anchor for this finding.", label)
	}
	return fmt.Sprintf("%s is connected to the issue anchor through the evidence graph or static call-site analysis.", label)
}

func callHierarchySourceLabel(source string) string {
	switch source {
	case "go_ast_call_scan":
		return "local Go AST call-site analysis"
	case "heuristic_call_scan":
		return "bundled heuristic call-site scan"
	case "gopls_call_hierarchy":
		return "gopls call hierarchy"
	default:
		return "static call-site analysis"
	}
}

func callPathStepShortLabel(step dbgen.CallPathStep) string {
	label := strings.TrimSpace(step.Label)
	if label != "" {
		return label
	}
	path := nullableStringValue(step.Path)
	line := nullableInt64Value(step.StartLine)
	if path != "" {
		return lineLabel(path, line, nullableInt64Value(step.EndLine))
	}
	return "step"
}

func layoutNarrative(layout json.RawMessage) ([]string, string, string) {
	var payload struct {
		MissingReasons            []string `json:"missing_reasons"`
		CallPathUnavailableReason string   `json:"call_path_unavailable_reason"`
		ConnectionSummary         string   `json:"connection_summary"`
	}
	if err := json.Unmarshal(layout, &payload); err != nil {
		return nil, "", ""
	}
	return payload.MissingReasons, strings.TrimSpace(payload.CallPathUnavailableReason), strings.TrimSpace(payload.ConnectionSummary)
}

func metadataJSONString(raw json.RawMessage, key string) string {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func metadataString(raw string, key string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func defaultLegend() []LegendItem {
	return []LegendItem{
		{Kind: NodeChangedCode, Label: "Issue line", Description: "Primary reviewed code location where the finding is anchored."},
		{Kind: NodeEntrypoint, Label: "Entrypoint", Description: "Caller or entry path that reaches the issue line."},
		{Kind: NodeRelatedCode, Label: "Related code", Description: "Supporting repository context or evidence."},
		{Kind: NodeGuard, Label: "Guard", Description: "Authentication, authorization, or validation guard."},
		{Kind: NodeTest, Label: "Test signal", Description: "Related tests to inspect before accepting or dismissing the finding."},
		{Kind: NodeCounterEvidence, Label: "Verified contradiction", Description: "Evidence that directly weakens or contradicts the finding."},
		{Kind: NodeMissingGuard, Label: "Missing guard", Description: "Expected guard or validation relationship is absent."},
		{Kind: NodeUnknown, Label: "Unknown", Description: "Incomplete evidence or unavailable code relationship."},
	}
}

func lineLabel(path string, startLine int64, endLine int64) string {
	label := filepath.ToSlash(path)
	if startLine <= 0 {
		return label
	}
	if endLine > startLine {
		return fmt.Sprintf("%s:%d-%d", label, startLine, endLine)
	}
	return fmt.Sprintf("%s:%d", label, startLine)
}

func normalizeNodeKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case NodeChangedCode, NodeEntrypoint, NodeRoute, NodeRelatedCode, NodeMiddleware, NodeGuard, NodeHandler, NodeTest, NodeConfig, NodeCounterEvidence, NodeMissingGuard, NodeUnknown:
		return strings.TrimSpace(kind)
	default:
		return NodeUnknown
	}
}

func normalizeEdgeKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case EdgeCalls, EdgeMounts, EdgeProtects, EdgeTests, EdgeSupports, EdgeContradicts, EdgeMissingGuard, "imports", "reads", "writes", EdgeUnknown:
		return strings.TrimSpace(kind)
	default:
		return EdgeUnknown
	}
}

func normalizeEdgeStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return EdgeStatusObserved
	}
	return status
}

func trimOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func truncateText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func nullableInt64Value(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func stableMapID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	sum := hash.Sum(nil)
	return prefix + hex.EncodeToString(sum[:12])
}
