package evidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	GraphStatusReady   = "ready"
	GraphStatusPartial = "partial"
)

const (
	NodeChangedCode     = "changed_code"
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
	Path           string          `json:"path,omitempty"`
	Symbol         string          `json:"symbol,omitempty"`
	StartLine      int64           `json:"start_line,omitempty"`
	EndLine        int64           `json:"end_line,omitempty"`
	EvidenceItemID string          `json:"evidence_item_id,omitempty"`
	Confidence     float64         `json:"confidence"`
	DeepLink       *NodeDeepLink   `json:"deep_link,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
}

type NodeDeepLink struct {
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	StartLine int64  `json:"start_line,omitempty"`
	EndLine   int64  `json:"end_line,omitempty"`
}

type EdgeView struct {
	ID         string          `json:"id"`
	Source     string          `json:"source"`
	Target     string          `json:"target"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	Label      string          `json:"label,omitempty"`
	Confidence float64         `json:"confidence"`
	Metadata   json.RawMessage `json:"metadata"`
}

type CallPathView struct {
	ID         string             `json:"id"`
	Label      string             `json:"label,omitempty"`
	Confidence float64            `json:"confidence"`
	Steps      []CallPathStepView `json:"steps"`
}

type CallPathStepView struct {
	ID        string `json:"id,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	StepIndex int64  `json:"step_index"`
	Path      string `json:"path,omitempty"`
	StartLine int64  `json:"start_line,omitempty"`
	EndLine   int64  `json:"end_line,omitempty"`
	Label     string `json:"label"`
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
	EvidenceSummary        string                `json:"evidence_summary,omitempty"`
	CounterEvidenceSummary string                `json:"counter_evidence_summary,omitempty"`
	EvidenceCounts         map[string]int        `json:"evidence_counts"`
	Evidence               []MapPanelEvidenceRef `json:"evidence"`
}

type MapPanelEvidenceRef struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	Path       string  `json:"path,omitempty"`
	StartLine  int64   `json:"start_line,omitempty"`
	EndLine    int64   `json:"end_line,omitempty"`
	Confidence float64 `json:"confidence"`
}

type mapBuildPlan struct {
	status                    string
	summary                   string
	layout                    map[string]any
	missingReasons            []string
	callPathUnavailableReason string
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
	summary := MapSummary{Findings: len(findings), ByStatus: map[string]int{}}
	for _, finding := range findings {
		view, err := s.RebuildEvidenceMap(ctx, finding)
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
	view, err := s.LoadEvidenceMap(ctx, finding)
	if err == nil {
		return view, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MapView{}, false, err
	}
	view, err = s.RebuildEvidenceMap(ctx, finding)
	if err != nil {
		return MapView{}, false, err
	}
	return view, true, nil
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

	plan := buildMapPlan(finding, items)
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

func buildMapPlan(finding dbgen.Finding, items []dbgen.EvidenceItem) mapBuildPlan {
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
			metadata:   map[string]any{"source": "local_rule", "rule": missing.metadata["rule"]},
		})
	}
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
	plan.summary = mapSummaryText(finding, plan)
	plan.layout = map[string]any{
		"direction":                    "LR",
		"generated_by":                 "local_evidence_map_builder",
		"missing_reasons":              plan.missingReasons,
		"call_path_unavailable_reason": plan.callPathUnavailableReason,
		"omitted_evidence_items":       plan.omittedEvidenceItems,
		"limits": map[string]any{
			"evidence_items":  defaultEvidenceMapItemLimit,
			"call_path_steps": defaultCallPathStepLimit,
		},
	}
	return plan
}

func primaryNodeSpec(finding dbgen.Finding, primaryEvidence *dbgen.EvidenceItem) nodeSpec {
	path := nullableStringValue(finding.PrimaryPath)
	startLine := nullableInt64Value(finding.PrimaryStartLine)
	endLine := nullableInt64Value(finding.PrimaryEndLine)
	if endLine == 0 {
		endLine = startLine
	}
	label := "Primary changed code"
	if path != "" {
		label = lineLabel(path, startLine, endLine)
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
			"source": "finding_location",
			"role":   "primary",
		},
	}
	if primaryEvidence != nil {
		spec.evidenceItemID = primaryEvidence.ID
		spec.confidence = clampConfidence(primaryEvidence.Confidence)
		spec.metadata["evidence_item_id"] = primaryEvidence.ID
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
			label:      "counter-evidence",
			confidence: clampConfidence(item.Confidence),
			metadata:   map[string]any{"source": "evidence_item", "evidence_item_id": item.ID},
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
			metadata:   map[string]any{"source": "evidence_item", "evidence_item_id": item.ID},
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
			metadata:   map[string]any{"source": "evidence_item", "evidence_item_id": item.ID},
		}
	case KindSupporting, KindSearch, KindAgent, KindStaticAnalysis, KindNeutral:
		return edgeSpec{
			key:        "support:" + item.ID,
			sourceKey:  nodeKey,
			targetKey:  primaryKey,
			kind:       EdgeSupports,
			status:     EdgeStatusObserved,
			label:      "supports finding",
			confidence: clampConfidence(item.Confidence),
			metadata:   map[string]any{"source": "evidence_item", "evidence_item_id": item.ID},
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
			"source": "local_rule",
			"rule":   profile.ID,
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
	for _, node := range nodes {
		nodeViews = append(nodeViews, nodeView(node))
	}
	edgeViews := make([]EdgeView, 0, len(edges))
	for _, edge := range edges {
		edgeViews = append(edgeViews, edgeView(edge))
	}
	callPathViews := make([]CallPathView, 0, len(paths))
	for _, path := range paths {
		steps, err := s.Queries.ListCallPathStepsByCallPath(ctx, path.ID)
		if err != nil {
			return MapView{}, fmt.Errorf("list evidence map call path steps: %w", err)
		}
		callPathViews = append(callPathViews, callPathView(path, steps))
	}
	var primaryCallPath []CallPathStepView
	if len(callPathViews) > 0 {
		primaryCallPath = callPathViews[0].Steps
	}
	missingReasons, callPathReason := layoutReasons(layout)
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
			EvidenceSummary:        nullableStringValue(finding.EvidenceSummary),
			CounterEvidenceSummary: nullableStringValue(finding.CounterEvidenceSummary),
			EvidenceCounts:         evidenceItemKindCounts(items),
			Evidence:               panelEvidenceItems(items),
		},
		MissingReasons: missingReasons,
	}, nil
}

func nodeView(row dbgen.EvidenceNode) NodeView {
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
	return NodeView{
		ID:             row.ID,
		Kind:           row.Kind,
		Label:          row.Label,
		Path:           path,
		Symbol:         nullableStringValue(row.Symbol),
		StartLine:      startLine,
		EndLine:        endLine,
		EvidenceItemID: nullableStringValue(row.EvidenceItemID),
		Confidence:     row.Confidence,
		DeepLink:       deepLink,
		Metadata:       metadata,
	}
}

func edgeView(row dbgen.EvidenceEdge) EdgeView {
	metadata := json.RawMessage(row.MetadataJson)
	if len(metadata) == 0 || !json.Valid(metadata) {
		metadata = json.RawMessage("{}")
	}
	return EdgeView{
		ID:         row.ID,
		Source:     row.SourceNodeID,
		Target:     row.TargetNodeID,
		Kind:       row.Kind,
		Status:     row.Status,
		Label:      nullableStringValue(row.Label),
		Confidence: row.Confidence,
		Metadata:   metadata,
	}
}

func callPathView(row dbgen.CallPath, steps []dbgen.CallPathStep) CallPathView {
	view := CallPathView{
		ID:         row.ID,
		Label:      nullableStringValue(row.Label),
		Confidence: row.Confidence,
		Steps:      make([]CallPathStepView, 0, len(steps)),
	}
	for _, step := range steps {
		view.Steps = append(view.Steps, CallPathStepView{
			ID:        step.ID,
			NodeID:    nullableStringValue(step.NodeID),
			StepIndex: step.StepIndex,
			Path:      nullableStringValue(step.Path),
			StartLine: nullableInt64Value(step.StartLine),
			EndLine:   nullableInt64Value(step.EndLine),
			Label:     step.Label,
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

func panelEvidenceItems(items []dbgen.EvidenceItem) []MapPanelEvidenceRef {
	result := make([]MapPanelEvidenceRef, 0, len(items))
	for _, item := range items {
		result = append(result, MapPanelEvidenceRef{
			ID:         item.ID,
			Kind:       item.Kind,
			Title:      item.Title,
			Summary:    item.Summary,
			Path:       nullableStringValue(item.Path),
			StartLine:  nullableInt64Value(item.StartLine),
			EndLine:    nullableInt64Value(item.EndLine),
			Confidence: item.Confidence,
		})
	}
	return result
}

func boundedEvidenceItemsForGraph(items []dbgen.EvidenceItem, primary *dbgen.EvidenceItem) ([]dbgen.EvidenceItem, int) {
	filtered := make([]dbgen.EvidenceItem, 0, len(items))
	for _, item := range items {
		if primary != nil && item.ID == primary.ID {
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

func evidenceMapKindRank(kind string) int {
	switch kind {
	case KindSupporting:
		return 0
	case KindCounter:
		return 1
	case KindTest:
		return 2
	case KindMissing:
		return 3
	case KindSearch:
		return 4
	case KindStaticAnalysis:
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
	switch item.Kind {
	case KindCounter:
		return NodeCounterEvidence
	case KindTest:
		return NodeTest
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
	case containsAny(path, "handler", "controller", "route", "routes") || containsAny(text, "handler", "route"):
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

func layoutReasons(layout json.RawMessage) ([]string, string) {
	var payload struct {
		MissingReasons            []string `json:"missing_reasons"`
		CallPathUnavailableReason string   `json:"call_path_unavailable_reason"`
	}
	if err := json.Unmarshal(layout, &payload); err != nil {
		return nil, ""
	}
	return payload.MissingReasons, strings.TrimSpace(payload.CallPathUnavailableReason)
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
		{Kind: NodeChangedCode, Label: "Changed code", Description: "Primary reviewed code location."},
		{Kind: NodeRelatedCode, Label: "Related code", Description: "Supporting repository context or evidence."},
		{Kind: NodeGuard, Label: "Guard", Description: "Authentication, authorization, or validation guard."},
		{Kind: NodeTest, Label: "Test", Description: "Test coverage signal."},
		{Kind: NodeCounterEvidence, Label: "Counter-evidence", Description: "Evidence that weakens or contradicts the finding."},
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
	case NodeChangedCode, NodeRelatedCode, NodeMiddleware, NodeGuard, NodeHandler, NodeTest, NodeConfig, NodeCounterEvidence, NodeMissingGuard, NodeUnknown:
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
