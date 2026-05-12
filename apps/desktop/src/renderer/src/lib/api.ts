export type HttpMethod = "GET" | "POST" | "PATCH" | "DELETE";

export type QueryValue =
  | string
  | number
  | boolean
  | null
  | undefined
  | readonly (string | number | boolean)[];

export interface BackendConnection {
  baseUrl: string;
  authToken: string;
}

export interface ApiClientOptions extends BackendConnection {
  fetch?: FetchLike;
}

export interface ApiRequestOptions {
  method?: HttpMethod;
  body?: unknown;
  query?: Record<string, QueryValue>;
  headers?: HeadersInit;
  signal?: AbortSignal;
}

export interface ApiEnvelope<T> {
  data: T | null;
  error: ApiErrorBody | null;
  request_id?: string;
}

export interface ApiErrorBody {
  code?: string;
  message?: string;
  details?: unknown;
}

export interface ApiSessionResponse {
  status: "authenticated";
}

export interface ApiVersionResponse {
  service: string;
  version: string;
  data_dir: string;
}

export interface Workspace {
  id: string;
  name: string;
  root_path: string;
  default_repo_id: string | null;
  settings_json?: string;
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Repository {
  id: string;
  workspace_id: string;
  name: string;
  owner: string | null;
  remote_url: string | null;
  local_path: string;
  default_branch: string | null;
  created_at: string;
  updated_at: string;
}

export interface RepositoryBranch {
  name: string;
  commit_sha: string;
  upstream?: string;
  current: boolean;
  remote: boolean;
}

export interface OpenRepositoryResponse {
  workspace: Workspace;
  repository: Repository;
  repositories: Repository[];
}

export interface Snapshot {
  id: string;
  repository_id: string;
  source_type: string;
  provider?: string;
  owner?: string;
  repo?: string;
  pr_number?: number;
  pr_title?: string;
  pr_url?: string;
  base_ref?: string;
  head_ref?: string;
  base_sha?: string;
  head_sha?: string;
  diff_artifact_id?: string;
  previous_comments_artifact_id?: string;
  metadata: unknown;
  changed_file_count?: number;
}

export interface ChangedFile {
  id: string;
  snapshot_id: string;
  path: string;
  old_path?: string;
  status: string;
  additions: number;
  deletions: number;
  is_binary: boolean;
  is_generated: boolean;
  is_excluded: boolean;
  line_ranges: unknown;
  patch_artifact_id?: string;
}

export interface ChangedFilePatch {
  changed_file_id: string;
  artifact_id: string;
  content: string;
  content_type: string;
  size_bytes: number;
  content_truncated: boolean;
}

export interface AgentCapabilities {
  can_read?: boolean;
  can_cancel?: boolean;
  can_write?: boolean;
  can_publish?: boolean;
  supports_json?: boolean;
  supports_sessions?: boolean;
  supports_streaming?: boolean;
  output_modes?: string[];
  metadata?: Record<string, unknown>;
  [key: string]: unknown;
}

interface AgentDefinition {
  name: string;
  role: string;
  adapter_kind: string;
  command: string;
  args: string[];
  cwd_mode: string;
  env_allowlist: string[];
  output_mode: string;
  model_label?: string;
  reasoning_label?: string;
  settings: Record<string, unknown>;
  capabilities: AgentCapabilities;
}

export interface AgentPreset extends AgentDefinition {
  id: string;
  description: string;
  enabled: boolean;
}

export interface AgentConfig extends AgentDefinition {
  id: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AgentConfigInput {
  name: string;
  role: string;
  adapter_kind: string;
  command: string;
  args: string[];
  cwd_mode: string;
  env_allowlist: string[];
  output_mode: string;
  model_label?: string;
  reasoning_label?: string;
  capabilities: AgentCapabilities;
  settings: Record<string, unknown>;
  enabled?: boolean;
}

export type UpdateAgentConfigInput = Partial<AgentConfigInput>;

export interface AgentConfigHealth {
  agent_config_id: string;
  status: "available" | "degraded" | "unavailable" | "unknown";
  message: string;
  checked_at: string;
  capabilities: AgentCapabilities;
  metadata: Record<string, unknown>;
}

export interface AgentModelOption {
  id: string;
  label: string;
  provider: string;
  provider_label: string;
  source: "cli" | "cli-default" | "unavailable" | string;
  default: boolean;
  reasoning_efforts?: AgentReasoningOption[];
}

export interface AgentReasoningOption {
  id: string;
  label: string;
  default: boolean;
}

export interface AgentModelCatalog {
  provider: string;
  provider_label: string;
  command: string;
  available: boolean;
  source: "cli" | "cli-default" | "unavailable" | string;
  models: AgentModelOption[];
  cached_at?: string;
  expires_at?: string;
  stale?: boolean;
  refreshing?: boolean;
  error?: string;
}

export interface CredentialRef {
  id: string;
  kind: string;
  display_name: string;
  storage_provider: string;
  storage_key: string;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface GitHubCredentialStatusResponse {
  configured: boolean;
  credential?: CredentialRef;
}

export interface SaveGitHubCredentialRequest {
  display_name?: string;
  storage_key?: string;
  token: string;
}

export interface DeleteGitHubCredentialResponse {
  deleted: boolean;
  storage_key?: string;
}

export interface ReviewRule {
  id: string;
  workspace_id: string;
  scope: string;
  rule_type: string;
  content: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ReviewRuleListResponse {
  items: ReviewRule[];
}

export interface CreateReviewRuleRequest {
  scope?: string;
  rule_type?: string;
  content: string;
  enabled?: boolean;
}

export interface UpdateReviewRuleRequest {
  scope?: string;
  rule_type?: string;
  content?: string;
  enabled?: boolean;
}

export interface SetReviewRuleEnabledRequest {
  enabled: boolean;
}

export interface SettingsWorkspaceExport {
  name: string;
}

export interface SettingsAgentConfigExport {
  name: string;
  role: string;
  adapter_kind: string;
  command: string;
  args: string[];
  cwd_mode: string;
  env_allowlist: string[];
  output_mode: string;
  model_label?: string;
  reasoning_label?: string;
  capabilities: Record<string, unknown>;
  settings: Record<string, unknown>;
  enabled: boolean;
}

export interface SettingsReviewRuleExport {
  scope: string;
  rule_type: string;
  content: string;
  enabled: boolean;
}

export interface SettingsExportPayload {
  schema: "cocode.settings_export.v1";
  exported_at?: string;
  sections?: string[];
  workspace?: SettingsWorkspaceExport;
  workspace_settings: Record<string, unknown>;
  agent_presets?: AgentPreset[];
  agent_configs: SettingsAgentConfigExport[];
  review_rules: SettingsReviewRuleExport[];
}

export interface SettingsImportRequest {
  payload: SettingsExportPayload;
  collision_policy?: "skip" | "replace" | "rename" | "fail";
}

export interface SettingsImportReport {
  created: number;
  updated: number;
  skipped: number;
  collisions?: string[];
  redacted?: number;
}

export interface SettingsImportResponse {
  schema: string;
  imported_at: string;
  collision_policy: string;
  workspace_settings: SettingsImportReport;
  agent_configs: SettingsImportReport;
  review_rules: SettingsImportReport;
  warnings?: string[];
}

export interface ReviewSession {
  id: string;
  workspace_id: string;
  repository_id: string;
  snapshot_id: string;
  title: string;
  status: string;
  review_depth: string;
  context_policy: Record<string, unknown>;
  runtime_limit_seconds: number;
  focus_prompt?: string;
  agents: ReviewSessionAgent[];
  created_at: string;
  updated_at: string;
}

export interface DeleteReviewSessionResponse {
  id: string;
  deleted: boolean;
  snapshot_id: string;
  snapshot_deleted: boolean;
}

export interface ReviewContextPolicy {
  include_prompt_material?: boolean;
  include_changed_code?: boolean;
  include_related_call_sites?: boolean;
  include_related_tests?: boolean;
  include_project_conventions?: boolean;
  include_prior_comments?: boolean;
  include_prior_decisions?: boolean;
  redact_secrets?: boolean;
  local_only_paths?: string[];
  max_tokens?: number;
  max_items?: number;
}

export interface CreateReviewSessionRequest {
  workspace_id?: string;
  snapshot_id: string;
  title?: string;
  review_depth?: "quick" | "standard" | "deep";
  preset?: string;
  focus_prompt?: string;
  agent_config_ids: string[];
  agent_selections?: ReviewSessionAgentSelectionInput[];
  runtime_limit_seconds?: number;
  context_policy?: ReviewContextPolicy;
}

export interface ReviewSessionAgentSelectionInput {
  agent_config_id: string;
  role?: string;
  model_label?: string;
  reasoning_label?: string;
}

export interface ReviewSessionAgent {
  id: string;
  review_session_id: string;
  agent_config_id: string;
  role: string;
  run_order: number;
  enabled: boolean;
  settings_override: Record<string, unknown>;
}

export interface ReviewSessionSummary {
  review_session_id: string;
  status: string;
  phase?: string;
  phase_status?: string;
  progress_percent: number;
  changed_files_total: number;
  changed_files_scanned: number;
  agent_runs_total: number;
  active_agents: number;
  agent_status_counts: Record<string, number>;
  agent_runs?: AgentRunSummary[];
  finding_counts?: Record<string, unknown>;
}

export interface AgentRunSummary {
  id: string;
  review_session_id: string;
  agent_config_id: string;
  context_bundle_id?: string;
  status: string;
  role: string;
  started_at?: string;
  completed_at?: string;
  error_code?: string;
  error_message?: string;
}

export interface ReviewEvent {
  id: string;
  review_session_id: string;
  agent_run_id?: string;
  type: string;
  level: string;
  sequence: number;
  payload: Record<string, unknown>;
  artifact_id?: string;
  created_at: string;
}

export interface ReviewAuditLogResponse {
  entries: ReviewAuditLogEntry[];
}

export interface ReviewAuditLogEntry {
  id: string;
  kind: string;
  title: string;
  review_session_id: string;
  level?: string;
  status?: string;
  sequence?: number;
  finding_id?: string;
  agent_run_id?: string;
  artifact_id?: string;
  copy_packet_id?: string;
  publish_draft_id?: string;
  github_publication_id?: string;
  review_event?: string;
  created_at: string;
  metadata: Record<string, unknown>;
}

export interface Finding {
  id: string;
  review_session_id: string;
  canonical_claim: string;
  category: string;
  severity: string;
  confidence: number;
  verification_status: string;
  decision_status: string;
  primary_path?: string;
  primary_start_line?: number;
  primary_end_line?: number;
  evidence_summary?: string;
  counter_evidence_summary?: string;
  suggested_fix?: string;
  draft_comment?: string;
  fingerprint: string;
  merged_from_count: number;
  introduced_in_sha?: string;
  first_seen_at: string;
  updated_at: string;
  source_agents?: FindingSourceAgent[];
}

export interface FindingSourceAgent {
  agent_run_id: string;
  agent_config_id?: string;
  name: string;
  role?: string;
  adapter_kind?: string;
  model_label?: string;
  reasoning_label?: string;
  severity?: string;
  confidence?: number;
}

export interface FindingListStats {
  total: number;
  filtered: number;
  by_decision: Record<string, number>;
  by_severity: Record<string, number>;
  by_verification: Record<string, number>;
  needs_triage: number;
}

export interface FindingListFilterOption {
  id: string;
  label: string;
  count: number;
}

export interface FindingListFilters {
  agents: FindingListFilterOption[];
  files: FindingListFilterOption[];
}

export interface FindingListResponse {
  items: Finding[];
  stats: FindingListStats;
  filters: FindingListFilters;
}

export interface FindingCandidate {
  id: string;
  review_session_id: string;
  agent_run_id: string;
  raw_artifact_id?: string;
  category: string;
  severity: string;
  confidence: number;
  claim: string;
  primary_path?: string;
  primary_start_line?: number;
  primary_end_line?: number;
  locations: unknown;
  evidence: unknown;
  suggested_fix?: string;
  draft_comment?: string;
  fingerprint?: string;
  created_at: string;
  relation?: string;
  agent_config_id?: string;
  agent_name?: string;
  agent_role?: string;
  adapter_kind?: string;
  model_label?: string;
  reasoning_label?: string;
}

export interface EvidenceItem {
  id: string;
  finding_id: string;
  kind: string;
  title: string;
  summary: string;
  path?: string;
  start_line?: number;
  end_line?: number;
  artifact_id?: string;
  confidence: number;
  code_snippet?: string;
  line_window?: { start_line: number; end_line: number };
  metadata: unknown;
  created_at: string;
}

export interface EvidenceGroups {
  supporting: EvidenceItem[];
  counter: EvidenceItem[];
  neutral: EvidenceItem[];
  missing: EvidenceItem[];
  test: EvidenceItem[];
  search: EvidenceItem[];
  agent: EvidenceItem[];
  static_analysis: EvidenceItem[];
}

export interface HumanDecision {
  id: string;
  finding_id: string;
  review_session_id: string;
  decision: string;
  reason?: string;
  metadata: unknown;
  created_at: string;
}

export interface FindingDetailResponse {
  finding: Finding;
  candidates: FindingCandidate[];
  evidence_items: EvidenceItem[];
  evidence_groups: EvidenceGroups;
  decisions: HumanDecision[];
}

export interface EvidenceMapGraph {
  id: string;
  finding_id: string;
  review_session_id: string;
  status: string;
  summary?: string;
  layout: unknown;
  created_at: string;
  updated_at: string;
}

export interface EvidenceMapFinding {
  id: string;
  review_session_id: string;
  canonical_claim: string;
  category: string;
  severity: string;
  confidence: number;
  verification_status: string;
  decision_status: string;
  primary_path?: string;
  primary_start_line?: number;
  primary_end_line?: number;
  evidence_summary?: string;
  counter_evidence_summary?: string;
  suggested_fix?: string;
}

export interface EvidenceMapHierarchyItem {
  path: string;
  kind: string;
  start_line?: number;
  end_line?: number;
  node_ids: string[];
  evidence_item_ids?: string[];
}

export interface EvidenceMapNodeDeepLink {
  kind: string;
  path: string;
  start_line?: number;
  end_line?: number;
}

export interface EvidenceMapNode {
  id: string;
  kind: string;
  label: string;
  path?: string;
  symbol?: string;
  start_line?: number;
  end_line?: number;
  evidence_item_id?: string;
  confidence: number;
  deep_link?: EvidenceMapNodeDeepLink;
  metadata: unknown;
}

export interface EvidenceMapEdge {
  id: string;
  source: string;
  target: string;
  kind: string;
  status: string;
  label?: string;
  confidence: number;
  metadata: unknown;
}

export interface EvidenceMapCallPathStep {
  id?: string;
  node_id?: string;
  step_index: number;
  path?: string;
  start_line?: number;
  end_line?: number;
  label: string;
}

export interface EvidenceMapCallPath {
  id: string;
  label?: string;
  confidence: number;
  steps: EvidenceMapCallPathStep[];
}

export interface EvidenceMapLegendItem {
  kind: string;
  label: string;
  description: string;
}

export interface EvidenceMapPanelEvidenceRef {
  id: string;
  kind: string;
  title: string;
  summary: string;
  path?: string;
  start_line?: number;
  end_line?: number;
  confidence: number;
}

export interface EvidenceMapPanel {
  claim: string;
  severity: string;
  verification_status: string;
  decision_status: string;
  evidence_summary?: string;
  counter_evidence_summary?: string;
  suggested_fix?: string;
  evidence_counts: Record<string, number>;
  evidence: EvidenceMapPanelEvidenceRef[];
}

export interface EvidenceMapResponse {
  graph: EvidenceMapGraph;
  finding: EvidenceMapFinding;
  hierarchy: EvidenceMapHierarchyItem[];
  nodes: EvidenceMapNode[];
  edges: EvidenceMapEdge[];
  call_path: EvidenceMapCallPathStep[];
  call_paths: EvidenceMapCallPath[];
  call_path_unavailable_reason?: string;
  legend: EvidenceMapLegendItem[];
  panel: EvidenceMapPanel;
  missing_reasons?: string[];
}

export interface EvidenceMapGraphRef {
  node_id?: string;
  edge_id?: string;
  call_path_id?: string;
  [key: string]: unknown;
}

export interface FindingThreadMessage {
  id: string;
  thread_id: string;
  role: string;
  agent_config_id?: string;
  content: string;
  evidence_refs: unknown;
  artifact_id?: string;
  created_at: string;
}

export interface FindingThread {
  id: string;
  finding_id: string;
  review_session_id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

export interface FindingThreadView {
  finding: Finding;
  thread: FindingThread;
  messages: FindingThreadMessage[];
}

export interface AskEvidenceMapQuestionRequest {
  question: string;
  agent_config_id?: string;
  context_policy?: ReviewContextPolicy;
  graph_refs?: EvidenceMapGraphRef[];
}

export interface AskFindingQuestionRequest {
  question: string;
  agent_config_id?: string;
  context_policy?: ReviewContextPolicy;
}

export interface AskFindingQuestionResponse {
  thread: FindingThreadView;
  user_message: FindingThreadMessage;
  assistant_message: FindingThreadMessage;
  agent_run_id?: string;
  context_bundle_id?: string;
}

export interface FindingQuickActionRequest {
  action: string;
  reason?: string;
  agent_config_id?: string;
  context_policy?: ReviewContextPolicy;
}

export interface FindingQuickActionResponse {
  action: string;
  thread: FindingThreadView;
  finding: Finding;
  decision?: HumanDecision;
  message?: FindingThreadMessage;
  assistant_message?: FindingThreadMessage;
  agent_run_id?: string;
  context_bundle_id?: string;
}

export interface ChatThread {
  id: string;
  review_session_id: string;
  title: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface ChatMessage {
  id: string;
  thread_id: string;
  parent_message_id?: string;
  author_type:
    | "user"
    | "cocode"
    | "orchestrator"
    | "agent"
    | "system"
    | "verifier"
    | string;
  author_display_name: string;
  agent_config_id?: string;
  agent_run_id?: string;
  context_bundle_id?: string;
  artifact_id?: string;
  body: string;
  status: "completed" | "failed" | string;
  metadata: unknown;
  created_at: string;
  updated_at: string;
}

export interface ChatTurn {
  id: string;
  thread_id: string;
  user_message_id: string;
  mode: string;
  audience: "orchestrator" | "all_agents" | "selected_agent" | string;
  responder_agent_config_id?: string;
  status: string;
  error_code?: string;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ChatThreadView {
  thread: ChatThread;
  messages: ChatMessage[];
}

export interface CreateChatTurnRequest {
  body: string;
  mode?: string;
  audience?: "orchestrator" | "all_agents" | "selected_agent";
  responder_agent_config_id?: string;
  context_refs?: unknown[];
  include_evidence?: boolean;
  include_recent_messages?: boolean;
}

export interface CreateChatTurnResponse extends ChatThreadView {
  turn: ChatTurn;
  agent_run_ids?: string[];
}

export interface CreateCopyPacketRequest {
  format?: string;
  finding_ids?: string[];
  include_code_snippets?: boolean;
  include_evidence?: boolean;
  include_counter_evidence?: boolean;
  target_agent?: string;
}

export interface CreateCopyPacketResponse {
  copy_packet_id: string;
  content: string;
  format: string;
  finding_count: number;
  token_estimate: number;
  content_artifact_id: string;
}

export interface MarkCopyPacketCopiedResponse {
  copy_packet_id: string;
  copied_at: string;
  finding_ids: string[];
  decisions: HumanDecision[];
}

export interface GitHubReviewCommentDraft {
  finding_id: string;
  path?: string;
  body: string;
  line?: number;
  side?: string;
  position?: number;
  unanchored: boolean;
  warning?: string;
}

export interface GitHubAnchorWarning {
  finding_id: string;
  path?: string;
  line?: number;
  message: string;
}

export interface GitHubPreviewChecklist {
  has_selected_findings: boolean;
  has_inline_comments: boolean;
  has_unanchored_comments: boolean;
  can_publish_inline: boolean;
  can_publish_summary_only: boolean;
}

export interface GitHubPreviewResponse {
  publish_draft_id: string;
  artifact_id: string;
  review_event: string;
  body: string;
  comments: GitHubReviewCommentDraft[];
  warnings: GitHubAnchorWarning[];
  checklist: GitHubPreviewChecklist;
}

export interface UpdateFindingDecisionRequest {
  decision: string;
  reason?: string;
  rule_memory_suggestion?: string;
}

export interface RedactionReport {
  bundle_id: string;
  redaction_count: number;
  items: RedactionReportItem[];
}

export interface RedactionReportItem {
  item_id: string;
  kind: string;
  path?: string;
  title?: string;
  redaction_count: number;
  detectors: Record<string, number>;
}

export interface VisibilityReport {
  recipient: {
    agent_config_id?: string;
    adapter_kind?: string;
    provider?: string;
    egress?: string;
    local_only?: boolean;
  };
  sent_item_count: number;
  sent_item_by_kind?: Record<string, number>;
  local_only_enforced?: boolean;
  local_only_paths?: string[];
  omitted?: VisibilityOmission[];
}

export interface VisibilityOmission {
  path?: string;
  item_id?: string;
  kind?: string;
  reason: string;
}

export interface ContextBundlePreview {
  bundle: {
    id: string;
    review_session_id: string;
    scope: string;
    token_estimate: number;
    item_count: number;
    items: unknown[];
  };
  dropped: unknown[];
  warnings?: string[];
  redaction_report: RedactionReport;
  visibility_report: VisibilityReport;
  persisted: boolean;
  artifact_id?: string;
  redaction_report_artifact_id?: string;
}

export type Loadable<T> =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "success"; data: T }
  | { status: "error"; error: ApiError };

type FetchLike = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: unknown;
  readonly requestId?: string;

  constructor(params: {
    message: string;
    status: number;
    code?: string;
    details?: unknown;
    requestId?: string;
    cause?: unknown;
  }) {
    super(params.message, { cause: params.cause });
    this.name = "ApiError";
    this.status = params.status;
    this.code = params.code ?? "API_ERROR";
    this.details = params.details;
    this.requestId = params.requestId;
  }
}

export class ApiClient {
  private readonly baseUrl: string;
  private readonly authToken: string;
  private readonly fetcher: FetchLike;

  constructor(options: ApiClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/+$/, "");
    this.authToken = options.authToken;
    this.fetcher = options.fetch ?? fetch.bind(globalThis);
  }

  session(options: Omit<ApiRequestOptions, "method" | "body"> = {}) {
    return this.get<ApiSessionResponse>("/api/session", options);
  }

  version(options: Omit<ApiRequestOptions, "method" | "body"> = {}) {
    return this.get<ApiVersionResponse>("/api/version", options);
  }

  listWorkspaces(options: Omit<ApiRequestOptions, "method" | "body"> = {}) {
    return this.get<Workspace[]>("/api/workspaces", options);
  }

  openRepository(
    path: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<OpenRepositoryResponse>(
      "/api/workspaces/open-repository",
      { path },
      options,
    );
  }

  listRepositories(
    workspaceId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<Repository[]>(
      `/api/workspaces/${encodeURIComponent(workspaceId)}/repositories`,
      options,
    );
  }

  listRepositoryBranches(
    repositoryId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> & {
      workspaceId?: string;
    } = {},
  ) {
    const { workspaceId, ...requestOptions } = options;
    return this.get<RepositoryBranch[]>(
      `/api/repositories/${encodeURIComponent(repositoryId)}/branches`,
      {
        ...requestOptions,
        query: {
          ...requestOptions.query,
          workspace_id: workspaceId,
        },
      },
    );
  }

  createGitHubSnapshot(
    body: {
      workspace_id: string;
      repository_id: string;
      url: string;
      github_token?: string;
      auth_method?: "token" | "api" | "gh_cli";
    },
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<Snapshot>(
      "/api/pr-snapshots/from-github-url",
      body,
      options,
    );
  }

  createLocalCompareSnapshot(
    body: {
      workspace_id: string;
      repository_id: string;
      base_ref: string;
      head_ref: string;
    },
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<Snapshot>(
      "/api/pr-snapshots/from-local-compare",
      body,
      options,
    );
  }

  createLocalChangesSnapshot(
    body: {
      workspace_id: string;
      repository_id: string;
    },
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<Snapshot>(
      "/api/pr-snapshots/from-local-changes",
      body,
      options,
    );
  }

  listChangedFiles(
    snapshotId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ChangedFile[]>(
      `/api/pr-snapshots/${encodeURIComponent(snapshotId)}/changed-files`,
      options,
    );
  }

  getChangedFilePatch(
    snapshotId: string,
    changedFileId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ChangedFilePatch>(
      `/api/pr-snapshots/${encodeURIComponent(snapshotId)}/changed-files/${encodeURIComponent(changedFileId)}/patch`,
      options,
    );
  }

  listAgentPresets(options: Omit<ApiRequestOptions, "method" | "body"> = {}) {
    return this.get<AgentPreset[]>("/api/agents/presets", options);
  }

  listAgentModelCatalog(
    options: Omit<ApiRequestOptions, "method" | "body"> & {
      refresh?: boolean;
    } = {},
  ) {
    const { refresh, ...requestOptions } = options;
    return this.get<AgentModelCatalog[]>("/api/agents/model-catalog", {
      ...requestOptions,
      query: { ...requestOptions.query, refresh: refresh ? 1 : undefined },
    });
  }

  listAgentConfigs(options: Omit<ApiRequestOptions, "method" | "body"> = {}) {
    return this.get<AgentConfig[]>("/api/agents/configs", options);
  }

  getGitHubCredential(
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<GitHubCredentialStatusResponse>(
      "/api/credentials/github",
      options,
    );
  }

  saveGitHubCredential(
    body: SaveGitHubCredentialRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<GitHubCredentialStatusResponse>(
      "/api/credentials/github",
      body,
      options,
    );
  }

  deleteGitHubCredential(
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.delete<DeleteGitHubCredentialResponse>(
      "/api/credentials/github",
      options,
    );
  }

  listReviewRules(
    workspaceId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ReviewRuleListResponse>(
      `/api/workspaces/${encodeURIComponent(workspaceId)}/review-rules`,
      options,
    );
  }

  createReviewRule(
    workspaceId: string,
    body: CreateReviewRuleRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<ReviewRule>(
      `/api/workspaces/${encodeURIComponent(workspaceId)}/review-rules`,
      body,
      options,
    );
  }

  updateReviewRule(
    id: string,
    body: UpdateReviewRuleRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.patch<ReviewRule>(
      `/api/review-rules/${encodeURIComponent(id)}`,
      body,
      options,
    );
  }

  setReviewRuleEnabled(
    id: string,
    body: SetReviewRuleEnabledRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.patch<ReviewRule>(
      `/api/review-rules/${encodeURIComponent(id)}/enabled`,
      body,
      options,
    );
  }

  deleteReviewRule(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.delete<{ deleted: boolean; id: string }>(
      `/api/review-rules/${encodeURIComponent(id)}`,
      options,
    );
  }

  exportWorkspaceSettings(
    workspaceId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<SettingsExportPayload>(
      `/api/workspaces/${encodeURIComponent(workspaceId)}/settings-export`,
      options,
    );
  }

  importWorkspaceSettings(
    workspaceId: string,
    body: SettingsImportRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<SettingsImportResponse>(
      `/api/workspaces/${encodeURIComponent(workspaceId)}/settings-import`,
      body,
      options,
    );
  }

  createAgentConfig(
    body: AgentConfigInput,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<AgentConfig>("/api/agents/configs", body, options);
  }

  updateAgentConfig(
    id: string,
    body: UpdateAgentConfigInput,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.patch<AgentConfig>(
      `/api/agents/configs/${encodeURIComponent(id)}`,
      body,
      options,
    );
  }

  deleteAgentConfig(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.delete<{ deleted: boolean }>(
      `/api/agents/configs/${encodeURIComponent(id)}`,
      options,
    );
  }

  testAgentConfig(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<AgentConfigHealth>(
      `/api/agents/configs/${encodeURIComponent(id)}/test`,
      undefined,
      options,
    );
  }

  listReviewSessions(
    workspaceId: string,
    options: Omit<ApiRequestOptions, "method" | "body" | "query"> = {},
  ) {
    return this.get<ReviewSession[]>("/api/review-sessions", {
      ...options,
      query: { workspace_id: workspaceId },
    });
  }

  createReviewSession(
    body: CreateReviewSessionRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<ReviewSession>("/api/review-sessions", body, options);
  }

  getReviewSession(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ReviewSession>(
      `/api/review-sessions/${encodeURIComponent(id)}`,
      options,
    );
  }

  deleteReviewSession(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.delete<DeleteReviewSessionResponse>(
      `/api/review-sessions/${encodeURIComponent(id)}`,
      options,
    );
  }

  reviewSessionSummary(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ReviewSessionSummary>(
      `/api/review-sessions/${encodeURIComponent(id)}/summary`,
      options,
    );
  }

  getReviewAuditLog(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ReviewAuditLogResponse>(
      `/api/review-sessions/${encodeURIComponent(id)}/audit-log`,
      options,
    );
  }

  startReviewSession(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<ReviewSession>(
      `/api/review-sessions/${encodeURIComponent(id)}/start`,
      undefined,
      options,
    );
  }

  pauseReviewSession(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<ReviewSession>(
      `/api/review-sessions/${encodeURIComponent(id)}/pause`,
      undefined,
      options,
    );
  }

  resumeReviewSession(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<ReviewSession>(
      `/api/review-sessions/${encodeURIComponent(id)}/resume`,
      undefined,
      options,
    );
  }

  cancelReviewSession(
    id: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<ReviewSession>(
      `/api/review-sessions/${encodeURIComponent(id)}/cancel`,
      undefined,
      options,
    );
  }

  cancelAgentRun(
    reviewSessionId: string,
    agentRunId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<AgentRunSummary>(
      `/api/review-sessions/${encodeURIComponent(reviewSessionId)}/agent-runs/${encodeURIComponent(agentRunId)}/cancel`,
      undefined,
      options,
    );
  }

  listFindings(
    reviewSessionId: string,
    query: {
      status?: string;
      severity?: string;
      agent?: string;
      file?: string;
      q?: string;
    } = {},
    options: Omit<ApiRequestOptions, "method" | "body" | "query"> = {},
  ) {
    return this.get<FindingListResponse>(
      `/api/review-sessions/${encodeURIComponent(reviewSessionId)}/findings`,
      { ...options, query },
    );
  }

  getFindingDetail(
    findingId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<FindingDetailResponse>(
      `/api/findings/${encodeURIComponent(findingId)}`,
      options,
    );
  }

  updateFindingDecision(
    findingId: string,
    body: UpdateFindingDecisionRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.patch<FindingDetailResponse>(
      `/api/findings/${encodeURIComponent(findingId)}/decision`,
      body,
      options,
    );
  }

  updateFindingDraftComment(
    findingId: string,
    draftComment: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.patch<Finding>(
      `/api/findings/${encodeURIComponent(findingId)}/draft-comment`,
      { draft_comment: draftComment },
      options,
    );
  }

  getFindingThread(
    findingId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<FindingThreadView>(
      `/api/findings/${encodeURIComponent(findingId)}/thread`,
      options,
    );
  }

  askFindingQuestion(
    findingId: string,
    body: AskFindingQuestionRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<AskFindingQuestionResponse>(
      `/api/findings/${encodeURIComponent(findingId)}/question`,
      body,
      options,
    );
  }

  runFindingQuickAction(
    findingId: string,
    body: FindingQuickActionRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<FindingQuickActionResponse>(
      `/api/findings/${encodeURIComponent(findingId)}/thread/actions`,
      body,
      options,
    );
  }

  createReviewCopyPacket(
    reviewSessionId: string,
    body: CreateCopyPacketRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<CreateCopyPacketResponse>(
      `/api/review-sessions/${encodeURIComponent(reviewSessionId)}/export/copy-packet`,
      body,
      options,
    );
  }

  markCopyPacketCopied(
    copyPacketId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<MarkCopyPacketCopiedResponse>(
      `/api/copy-packets/${encodeURIComponent(copyPacketId)}/copied`,
      undefined,
      options,
    );
  }

  createGitHubPreview(
    reviewSessionId: string,
    body: { finding_ids?: string[]; review_event?: string },
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<GitHubPreviewResponse>(
      `/api/review-sessions/${encodeURIComponent(reviewSessionId)}/github/preview`,
      body,
      options,
    );
  }

  getFindingEvidenceMap(
    findingId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<EvidenceMapResponse>(
      `/api/findings/${encodeURIComponent(findingId)}/evidence-map`,
      options,
    );
  }

  rebuildFindingEvidenceMap(
    findingId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<EvidenceMapResponse>(
      `/api/findings/${encodeURIComponent(findingId)}/evidence-map/rebuild`,
      undefined,
      options,
    );
  }

  askEvidenceMapQuestion(
    findingId: string,
    body: AskEvidenceMapQuestionRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<AskFindingQuestionResponse>(
      `/api/findings/${encodeURIComponent(findingId)}/evidence-map/question`,
      body,
      options,
    );
  }

  getReviewSessionChatThread(
    reviewSessionId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ChatThreadView>(
      `/api/review-sessions/${encodeURIComponent(reviewSessionId)}/chat-thread`,
      options,
    );
  }

  createReviewSessionChatTurn(
    reviewSessionId: string,
    body: CreateChatTurnRequest,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<CreateChatTurnResponse>(
      `/api/review-sessions/${encodeURIComponent(reviewSessionId)}/chat-turns`,
      body,
      options,
    );
  }

  getChatThread(
    threadId: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.get<ChatThreadView>(
      `/api/chat-threads/${encodeURIComponent(threadId)}`,
      options,
    );
  }

  async streamReviewEvents(
    id: string,
    options: {
      afterSequence?: number;
      signal?: AbortSignal;
      onEvent: (event: ReviewEvent) => void;
    },
  ) {
    const response = await this.fetcher(
      endpointUrl(
        this.baseUrl,
        `/api/review-sessions/${encodeURIComponent(id)}/events`,
        { after_sequence: options.afterSequence },
      ),
      {
        method: "GET",
        headers: requestHeaders(this.authToken, {}),
        signal: options.signal,
      },
    );
    if (!response.ok) {
      await parseEnvelopeResponse<never>(response);
      return;
    }
    if (!response.body) {
      throw new ApiError({
        message: "Review event stream is unavailable",
        status: response.status,
        code: "STREAM_UNAVAILABLE",
      });
    }
    await readReviewEventStream(response.body, options.onEvent);
  }

  previewReviewContext(
    id: string,
    body: {
      agent_config_id?: string;
      persist?: boolean;
      context_policy?: ReviewContextPolicy;
    },
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.post<ContextBundlePreview>(
      `/api/review-sessions/${encodeURIComponent(id)}/context-bundles/preview`,
      body,
      options,
    );
  }

  get<T>(
    path: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.request<T>(path, { ...options, method: "GET" });
  }

  post<T>(
    path: string,
    body?: unknown,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.request<T>(path, { ...options, method: "POST", body });
  }

  patch<T>(
    path: string,
    body?: unknown,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.request<T>(path, { ...options, method: "PATCH", body });
  }

  delete<T>(
    path: string,
    options: Omit<ApiRequestOptions, "method" | "body"> = {},
  ) {
    return this.request<T>(path, { ...options, method: "DELETE" });
  }

  async request<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
    const response = await this.fetcher(
      endpointUrl(this.baseUrl, path, options.query),
      {
        method: options.method ?? "GET",
        headers: requestHeaders(this.authToken, options),
        body: requestBody(options.body),
        signal: options.signal,
      },
    );
    return parseEnvelopeResponse<T>(response);
  }
}

export function createCocodeClient(options: ApiClientOptions): ApiClient {
  return new ApiClient(options);
}

export function idleApiState<T>(): Loadable<T> {
  return { status: "idle" };
}

export function loadingApiState<T>(): Loadable<T> {
  return { status: "loading" };
}

export function successApiState<T>(data: T): Loadable<T> {
  return { status: "success", data };
}

export function errorApiState<T>(error: unknown): Loadable<T> {
  return { status: "error", error: toApiError(error) };
}

export function preserveSuccessfulLoadable<T>(
  current: Loadable<T>,
  next: Loadable<T>,
): Loadable<T> {
  if (current.status === "success" && next.status === "error") {
    return current;
  }
  return next;
}

export async function loadApiResource<T>(
  loader: () => Promise<T>,
): Promise<Loadable<T>> {
  try {
    return successApiState(await loader());
  } catch (error) {
    return errorApiState(error);
  }
}

export function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) {
    return error;
  }
  if (error instanceof Error) {
    return new ApiError({
      message: error.message,
      status: 0,
      code: "NETWORK_ERROR",
      cause: error,
    });
  }
  return new ApiError({
    message: String(error),
    status: 0,
    code: "UNKNOWN_ERROR",
  });
}

async function parseEnvelopeResponse<T>(response: Response): Promise<T> {
  const payload = await readJSON(response);
  if (!isApiEnvelope<T>(payload)) {
    throw new ApiError({
      message: "Backend response envelope is invalid",
      status: response.status,
      code: "INVALID_ENVELOPE",
      details: payload,
    });
  }
  if (!response.ok || payload.error) {
    throw new ApiError({
      message:
        payload.error?.message ??
        `Request failed with status ${response.status}`,
      status: response.status,
      code: payload.error?.code,
      details: payload.error?.details,
      requestId: payload.request_id,
    });
  }
  return payload.data as T;
}

async function readJSON(response: Response): Promise<unknown> {
  const text = await response.text();
  if (text.trim() === "") {
    return null;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch (error) {
    throw new ApiError({
      message: "Backend response was not valid JSON",
      status: response.status,
      code: "INVALID_JSON",
      cause: error,
    });
  }
}

async function readReviewEventStream(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: ReviewEvent) => void,
) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    buffer = consumeSSEBuffer(buffer, onEvent);
  }

  buffer += decoder.decode();
  consumeSSEBuffer(buffer + "\n\n", onEvent);
}

function consumeSSEBuffer(
  buffer: string,
  onEvent: (event: ReviewEvent) => void,
) {
  buffer = buffer.replace(/\r\n/g, "\n");
  let separator = buffer.indexOf("\n\n");
  while (separator >= 0) {
    const block = buffer.slice(0, separator);
    emitSSEBlock(block, onEvent);
    buffer = buffer.slice(separator + 2);
    separator = buffer.indexOf("\n\n");
  }
  return buffer;
}

function emitSSEBlock(block: string, onEvent: (event: ReviewEvent) => void) {
  let eventName = "message";
  const data: string[] = [];

  for (const line of block.split("\n")) {
    if (line.startsWith(":") || line.trim() === "") {
      continue;
    }
    const separator = line.indexOf(":");
    const field = separator >= 0 ? line.slice(0, separator) : line;
    const rawValue = separator >= 0 ? line.slice(separator + 1) : "";
    const value = rawValue.startsWith(" ") ? rawValue.slice(1) : rawValue;
    if (field === "event") {
      eventName = value;
    }
    if (field === "data") {
      data.push(value);
    }
  }

  if (eventName !== "review.event" || data.length === 0) {
    return;
  }

  try {
    onEvent(JSON.parse(data.join("\n")) as ReviewEvent);
  } catch (error) {
    throw new ApiError({
      message: "Review event stream emitted invalid JSON",
      status: 200,
      code: "INVALID_SSE_EVENT",
      cause: error,
    });
  }
}

function isApiEnvelope<T>(value: unknown): value is ApiEnvelope<T> {
  return (
    typeof value === "object" &&
    value !== null &&
    "data" in value &&
    "error" in value
  );
}

function endpointUrl(
  baseUrl: string,
  path: string,
  query?: Record<string, QueryValue>,
): URL {
  const url = new URL(path.startsWith("/") ? path : `/${path}`, `${baseUrl}/`);
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value === null || value === undefined) {
      continue;
    }
    if (Array.isArray(value)) {
      for (const item of value) {
        url.searchParams.append(key, String(item));
      }
      continue;
    }
    url.searchParams.set(key, String(value));
  }
  return url;
}

function requestHeaders(
  authToken: string,
  options: ApiRequestOptions,
): Headers {
  const headers = new Headers(options.headers);
  if (!headers.has("Accept")) {
    headers.set("Accept", "application/json");
  }
  if (authToken !== "" && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${authToken}`);
  }
  if (options.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  return headers;
}

function requestBody(body: unknown): BodyInit | undefined {
  if (body === undefined) {
    return undefined;
  }
  if (
    typeof body === "string" ||
    body instanceof FormData ||
    body instanceof Blob
  ) {
    return body;
  }
  return JSON.stringify(body);
}
