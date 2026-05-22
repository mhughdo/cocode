import {
  ArrowDownIcon,
  ArrowUpIcon,
  BookOpenIcon,
  CheckIcon,
  CopyIcon,
  GitPullRequestIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react";

import {
  EmptyState,
  ErrorState,
  LoadingRows,
  TooltipIconButton,
} from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  type AgentConfigHealth,
  type DeleteGitHubCredentialResponse,
  type GitHubCredentialStatusResponse,
  type Loadable,
  type ReviewRule,
  type ReviewRuleListResponse,
  type SettingsExportPayload,
  type SettingsImportResponse,
  successApiState,
  type Workspace,
} from "@/lib/api";
import { cn } from "@/lib/utils";

import { formatRelativeAge } from "../shared/time-format";

export type ReviewRuleDraftState = {
  scope: string;
  ruleType: string;
  content: string;
  enabled: boolean;
};

export type SettingsCollisionPolicy = "skip" | "replace" | "rename" | "fail";

export function GitHubCredentialPanel({
  deleteState,
  displayName,
  saveState,
  status,
  token,
  onDelete,
  onDisplayNameChange,
  onSave,
  onTokenChange,
}: {
  deleteState: Loadable<DeleteGitHubCredentialResponse>;
  displayName: string;
  saveState: Loadable<GitHubCredentialStatusResponse>;
  status: Loadable<GitHubCredentialStatusResponse>;
  token: string;
  onDelete: () => void;
  onDisplayNameChange: (value: string) => void;
  onSave: () => void;
  onTokenChange: (value: string) => void;
}) {
  const credential =
    status.status === "success" && status.data.configured
      ? status.data.credential
      : undefined;
  const metadata = credential?.metadata ?? {};
  const login = typeof metadata.login === "string" ? metadata.login : "";
  const scopes = Array.isArray(metadata.scopes)
    ? metadata.scopes.filter(
        (scope): scope is string => typeof scope === "string",
      )
    : [];
  const validatedAt =
    typeof metadata.validated_at === "string" ? metadata.validated_at : "";
  const isSaving = saveState.status === "loading";
  const isDeleting = deleteState.status === "loading";

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <GitPullRequestIcon className="size-4" />
            GitHub credentials
          </div>
          <div className="text-muted-foreground mt-1 text-xs">
            Token value is encrypted by the desktop safe store; cocoded keeps
            only a credential reference.
          </div>
        </div>
        {status.status === "loading" ? (
          <Badge variant="outline">checking</Badge>
        ) : credential ? (
          <Badge variant="secondary">configured</Badge>
        ) : (
          <Badge variant="outline">missing</Badge>
        )}
      </div>

      <div className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.7fr)]">
        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
          <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
            Display name
            <Input
              placeholder="GitHub token"
              value={displayName}
              onChange={(event) => onDisplayNameChange(event.target.value)}
            />
          </label>
          <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
            Token
            <Input
              placeholder="ghp_..."
              type="password"
              value={token}
              onChange={(event) => onTokenChange(event.target.value)}
            />
          </label>
          <div className="flex flex-wrap items-center gap-2 sm:col-span-2">
            <Button disabled={isSaving || !token.trim()} onClick={onSave}>
              <CheckIcon data-icon="inline-start" />
              {isSaving ? "Saving..." : "Save token"}
            </Button>
            <Button
              disabled={!credential || isDeleting}
              variant="outline"
              onClick={onDelete}
            >
              {isDeleting ? "Deleting..." : "Delete token"}
            </Button>
          </div>
          {saveState.status === "error" && (
            <ErrorState
              className="sm:col-span-2"
              title="Could not save GitHub token"
              description={saveState.error.message}
            />
          )}
          {deleteState.status === "error" && (
            <ErrorState
              className="sm:col-span-2"
              title="Could not delete GitHub token"
              description={deleteState.error.message}
            />
          )}
        </div>

        <div className="rounded-md border p-3">
          {status.status === "loading" && <LoadingRows rows={3} />}
          {status.status === "error" && (
            <ErrorState
              className="border-0 p-0"
              title="Credential status unavailable"
              description={status.error.message}
            />
          )}
          {status.status === "success" && !credential && (
            <EmptyState
              className="border-0 p-0"
              title="No GitHub token"
              description="GitHub PR ingestion will ask for a saved token before it calls the API."
              icon={GitPullRequestIcon}
            />
          )}
          {credential && (
            <div className="space-y-3 text-sm">
              <div>
                <div className="truncate font-medium">
                  {credential.display_name}
                </div>
                <div className="text-muted-foreground mt-1 truncate text-xs">
                  {login || credential.kind}
                  {validatedAt ? ` • ${formatRelativeAge(validatedAt)}` : ""}
                </div>
              </div>
              <div className="flex flex-wrap gap-1">
                {scopes.length > 0 ? (
                  scopes.slice(0, 6).map((scope) => (
                    <Badge key={scope} variant="outline">
                      {scope}
                    </Badge>
                  ))
                ) : (
                  <Badge variant="outline">no scopes reported</Badge>
                )}
              </div>
              <div className="text-muted-foreground truncate font-mono text-xs">
                {credential.storage_provider}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

export function ReviewRuleMemoryPanel({
  actionState,
  draft,
  rules,
  workspace,
  onCreate,
  onDelete,
  onDraftChange,
  onReload,
  onToggle,
}: {
  actionState: Loadable<ReviewRule | { deleted: boolean; id: string }>;
  draft: ReviewRuleDraftState;
  rules: Loadable<ReviewRuleListResponse>;
  workspace?: Workspace;
  onCreate: () => void;
  onDelete: (rule: ReviewRule) => void;
  onDraftChange: (draft: ReviewRuleDraftState) => void;
  onReload: () => void;
  onToggle: (rule: ReviewRule, enabled: boolean) => void;
}) {
  const items = rules.status === "success" ? rules.data.items : [];
  const isBusy = actionState.status === "loading";

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <BookOpenIcon className="size-4" />
            Review rule memory
          </div>
          <div className="text-muted-foreground mt-1 truncate text-xs">
            Dismissed findings can become local guidance for future review
            context.
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={items.length > 0 ? "secondary" : "outline"}>
            {items.length} rules
          </Badge>
          <TooltipIconButton
            disabled={!workspace || rules.status === "loading"}
            label="Refresh review rules"
            size="icon-sm"
            variant="ghost"
            onClick={onReload}
          >
            <RefreshCwIcon />
          </TooltipIconButton>
        </div>
      </div>

      {!workspace ? (
        <div className="p-4">
          <EmptyState
            className="border-0 p-0"
            title="No project selected"
            description="Open a project before managing local review guidance."
            icon={BookOpenIcon}
          />
        </div>
      ) : (
        <div className="grid gap-4 p-4 lg:grid-cols-[minmax(280px,0.65fr)_minmax(0,1fr)]">
          <div className="flex min-w-0 flex-col gap-3 rounded-md border p-3">
            <div className="text-sm font-medium">Add rule</div>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
                Scope
                <NativeSelect
                  value={draft.scope}
                  onChange={(event) =>
                    onDraftChange({ ...draft, scope: event.target.value })
                  }
                >
                  <NativeSelectOption value="workspace">
                    project
                  </NativeSelectOption>
                  <NativeSelectOption value="repository">
                    repository
                  </NativeSelectOption>
                  <NativeSelectOption value="path">path</NativeSelectOption>
                </NativeSelect>
              </label>
              <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
                Type
                <NativeSelect
                  value={draft.ruleType}
                  onChange={(event) =>
                    onDraftChange({ ...draft, ruleType: event.target.value })
                  }
                >
                  <NativeSelectOption value="dismissal">
                    dismissal
                  </NativeSelectOption>
                  <NativeSelectOption value="false_positive">
                    false_positive
                  </NativeSelectOption>
                  <NativeSelectOption value="review_guidance">
                    review_guidance
                  </NativeSelectOption>
                  <NativeSelectOption value="custom">custom</NativeSelectOption>
                </NativeSelect>
              </label>
            </div>
            <label className="flex min-w-0 flex-col gap-2 text-sm font-medium">
              Guidance
              <Textarea
                className="min-h-24 text-sm"
                maxLength={2000}
                placeholder="Do not report generated client snapshots unless runtime behavior changes."
                value={draft.content}
                onChange={(event) =>
                  onDraftChange({ ...draft, content: event.target.value })
                }
              />
            </label>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <label className="flex items-center gap-2 text-sm">
                <Switch
                  checked={draft.enabled}
                  size="sm"
                  onCheckedChange={(enabled) =>
                    onDraftChange({ ...draft, enabled })
                  }
                />
                Enabled
              </label>
              <Button
                disabled={isBusy || !draft.content.trim()}
                size="sm"
                onClick={onCreate}
              >
                <PlusIcon data-icon="inline-start" />
                Add rule
              </Button>
            </div>
            {actionState.status === "error" && (
              <ErrorState
                title="Rule update failed"
                description={actionState.error.message}
              />
            )}
          </div>

          <div className="min-w-0 rounded-md border">
            <div className="flex items-center justify-between gap-3 border-b px-3 py-2">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">
                  {workspace.name}
                </div>
                <div className="text-muted-foreground truncate text-xs">
                  Stored locally and injected only when prior decisions are
                  included.
                </div>
              </div>
            </div>
            <div className="flex max-h-80 flex-col gap-2 overflow-auto p-3">
              {rules.status === "loading" && <LoadingRows rows={4} />}
              {rules.status === "error" && (
                <ErrorState
                  className="border-0 p-0"
                  title="Rules unavailable"
                  description={rules.error.message}
                />
              )}
              {rules.status === "success" && items.length === 0 && (
                <EmptyState
                  className="border-0 p-0"
                  title="No remembered rules"
                  description="Dismissed findings can be saved as local guidance from the findings board."
                  icon={BookOpenIcon}
                />
              )}
              {items.slice(0, 100).map((rule) => (
                <div
                  key={rule.id}
                  className={cn(
                    "bg-background flex min-w-0 items-start gap-3 rounded-md border p-3",
                    !rule.enabled && "opacity-70",
                  )}
                >
                  <Switch
                    checked={rule.enabled}
                    className="mt-0.5"
                    disabled={isBusy}
                    size="sm"
                    onCheckedChange={(enabled) => onToggle(rule, enabled)}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="mb-2 flex flex-wrap items-center gap-1">
                      <Badge variant="outline">
                        {formatReviewRuleScope(rule.scope)}
                      </Badge>
                      <Badge variant="secondary">{rule.rule_type}</Badge>
                      {!rule.enabled && <Badge variant="outline">off</Badge>}
                    </div>
                    <p className="line-clamp-3 text-sm">{rule.content}</p>
                    <div className="text-muted-foreground mt-2 truncate text-xs">
                      Updated {formatRelativeAge(rule.updated_at)}
                    </div>
                  </div>
                  <TooltipIconButton
                    disabled={isBusy}
                    label="Delete rule"
                    size="icon-sm"
                    variant="ghost"
                    onClick={() => onDelete(rule)}
                  >
                    <Trash2Icon />
                  </TooltipIconButton>
                </div>
              ))}
              {items.length > 100 && (
                <div className="text-muted-foreground px-1 text-xs">
                  Showing 100 of {items.length} rules.
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </section>
  );
}

export function SettingsPortabilityPanel({
  collisionPolicy,
  exportText,
  importText,
  state,
  workspace,
  onCollisionPolicyChange,
  onExport,
  onImport,
  onImportTextChange,
}: {
  collisionPolicy: SettingsCollisionPolicy;
  exportText: string;
  importText: string;
  state: Loadable<SettingsExportPayload | SettingsImportResponse>;
  workspace?: Workspace;
  onCollisionPolicyChange: (policy: SettingsCollisionPolicy) => void;
  onExport: () => void;
  onImport: () => void;
  onImportTextChange: (value: string) => void;
}) {
  const isBusy = state.status === "loading";
  const importResult =
    state.status === "success" && isSettingsImportResponse(state.data)
      ? state.data
      : undefined;

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <CopyIcon className="size-4" />
            Settings portability
          </div>
          <div className="text-muted-foreground mt-1 truncate text-xs">
            Portable JSON excludes secrets, local paths, review artifacts, and
            credential refs.
          </div>
        </div>
        <Badge variant={workspace ? "secondary" : "outline"}>
          {workspace?.name ?? "no project"}
        </Badge>
      </div>

      <div className="grid gap-4 p-4 lg:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-3">
          <div className="flex items-center justify-between gap-3">
            <div className="text-sm font-medium">Export</div>
            <Button
              disabled={!workspace || isBusy}
              size="sm"
              onClick={onExport}
            >
              <ArrowUpIcon data-icon="inline-start" />
              Export JSON
            </Button>
          </div>
          <Textarea
            aria-label="Settings export JSON"
            className="min-h-56 font-mono text-xs"
            readOnly
            placeholder="Exported settings JSON appears here."
            value={exportText}
          />
        </div>

        <div className="flex min-w-0 flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="text-sm font-medium">Import</div>
            <div className="flex items-center gap-2">
              <NativeSelect
                className="w-28"
                value={collisionPolicy}
                onChange={(event) =>
                  onCollisionPolicyChange(
                    event.target.value as SettingsCollisionPolicy,
                  )
                }
              >
                <NativeSelectOption value="skip">skip</NativeSelectOption>
                <NativeSelectOption value="replace">replace</NativeSelectOption>
                <NativeSelectOption value="rename">rename</NativeSelectOption>
                <NativeSelectOption value="fail">fail</NativeSelectOption>
              </NativeSelect>
              <Button
                disabled={!workspace || isBusy || !importText.trim()}
                size="sm"
                variant="outline"
                onClick={onImport}
              >
                <ArrowDownIcon data-icon="inline-start" />
                Import
              </Button>
            </div>
          </div>
          <Textarea
            aria-label="Settings import JSON"
            className="min-h-56 font-mono text-xs"
            placeholder="Paste a cocode.settings_export.v1 JSON payload."
            value={importText}
            onChange={(event) => onImportTextChange(event.target.value)}
          />
        </div>
      </div>

      {(state.status === "error" || importResult) && (
        <div className="border-t px-4 py-3">
          {state.status === "error" && (
            <ErrorState
              title="Settings portability failed"
              description={state.error.message}
            />
          )}
          {importResult && (
            <div className="grid gap-2 sm:grid-cols-3">
              <SettingsImportReportChip
                label="Project"
                report={importResult.workspace_settings}
              />
              <SettingsImportReportChip
                label="Agents"
                report={importResult.agent_configs}
              />
              <SettingsImportReportChip
                label="Rules"
                report={importResult.review_rules}
              />
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function SettingsImportReportChip({
  label,
  report,
}: {
  label: string;
  report: SettingsImportResponse["agent_configs"];
}) {
  return (
    <div className="bg-background rounded-md border px-3 py-2 text-sm">
      <div className="font-medium">{label}</div>
      <div className="text-muted-foreground mt-1 text-xs">
        {report.created} created, {report.updated} updated, {report.skipped}{" "}
        skipped
        {report.redacted ? `, ${report.redacted} redacted` : ""}
      </div>
    </div>
  );
}

function isSettingsImportResponse(
  value: SettingsExportPayload | SettingsImportResponse,
): value is SettingsImportResponse {
  return "imported_at" in value;
}

export function AgentSettingSwitch({
  checked,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <label className="bg-background flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm">
      <span className="truncate">{label}</span>
      <Switch checked={checked} size="sm" onCheckedChange={onCheckedChange} />
    </label>
  );
}

export function HealthSummary({ health }: { health: AgentConfigHealth }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Badge
          variant={
            health.status === "unavailable" ? "destructive" : "secondary"
          }
        >
          {health.status}
        </Badge>
        {health.message && (
          <span className="text-muted-foreground min-w-0 truncate text-sm">
            {health.message}
          </span>
        )}
      </div>
      {formatHealthMetadata(health.metadata).length > 0 && (
        <div className="grid grid-cols-2 gap-2">
          {formatHealthMetadata(health.metadata).map(([key, value]) => (
            <div
              key={key}
              className="bg-background rounded-md border px-2 py-1"
            >
              <div className="text-muted-foreground text-xs">{key}</div>
              <div className="truncate font-mono text-xs">{value}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function defaultReviewRuleDraft(): ReviewRuleDraftState {
  return {
    scope: "workspace",
    ruleType: "dismissal",
    content: "",
    enabled: true,
  };
}

function formatReviewRuleScope(scope: string) {
  return scope === "workspace" ? "project" : scope;
}

export function upsertReviewRuleState(
  current: Loadable<ReviewRuleListResponse>,
  rule: ReviewRule,
): Loadable<ReviewRuleListResponse> {
  const items = current.status === "success" ? current.data.items : [];
  const exists = items.some((item) => item.id === rule.id);
  const next = exists
    ? items.map((item) => (item.id === rule.id ? rule : item))
    : [rule, ...items];
  return successApiState({ items: next });
}

export function removeReviewRuleState(
  current: Loadable<ReviewRuleListResponse>,
  id: string,
): Loadable<ReviewRuleListResponse> {
  if (current.status !== "success") {
    return successApiState({ items: [] });
  }
  return successApiState({
    items: current.data.items.filter((item) => item.id !== id),
  });
}

function formatHealthMetadata(metadata: Record<string, unknown>) {
  return ["version", "resolved_path", "path", "error"]
    .map((key) => [key, metadata[key]] as const)
    .filter(([, value]) => typeof value === "string" && value.trim())
    .map(([key, value]) => [key, value as string] as const);
}
