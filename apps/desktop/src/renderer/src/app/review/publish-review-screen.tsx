import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CopyIcon,
  FileSearchIcon,
  GitPullRequestIcon,
  InboxIcon,
  MessageSquareIcon,
} from "lucide-react";

import {
  EmptyState,
  ErrorState,
  LoadingRows,
  PaneHeader,
} from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  type ApiClient,
  type CreateCopyPacketResponse,
  type FindingListResponse,
  type GitHubPreviewResponse,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type ReviewSession,
} from "@/lib/api";
import { MarkdownMessage } from "../chat/markdown-message";
import { formatFindingLocation } from "../evidence/review-evidence-utils";

export function PublishReviewScreen({
  client,
  session,
}: {
  client: ApiClient | null;
  session?: ReviewSession;
}) {
  const [acceptedFindings, setAcceptedFindings] =
    useState<Loadable<FindingListResponse>>(idleApiState());
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [reviewEvent, setReviewEvent] = useState("COMMENT");
  const [previewState, setPreviewState] =
    useState<Loadable<GitHubPreviewResponse>>(idleApiState());
  const [copyPacketState, setCopyPacketState] =
    useState<Loadable<CreateCopyPacketResponse>>(idleApiState());
  const [actionMessage, setActionMessage] = useState("");
  const previewAutoKeyRef = useRef("");

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client || !session) {
        setAcceptedFindings(idleApiState());
        setSelectedIds(new Set());
        return;
      }
      setAcceptedFindings(loadingApiState());
      void loadApiResource(() => client.listFindings(session.id, {})).then(
        (state) => {
          if (canceled) {
            return;
          }
          setAcceptedFindings(state);
          if (state.status === "success") {
            setSelectedIds(
              new Set(
                state.data.items
                  .filter((finding) => finding.decision_status === "accepted")
                  .map((finding) => finding.id),
              ),
            );
          }
        },
      );
    });
    return () => {
      canceled = true;
    };
  }, [client, session?.id, session]);

  const findings = useMemo(
    () =>
      acceptedFindings.status === "success"
        ? acceptedFindings.data.items.filter(
            (finding) => finding.decision_status === "accepted",
          )
        : [],
    [acceptedFindings],
  );
  const selectedFindings = useMemo(
    () => findings.filter((finding) => selectedIds.has(finding.id)),
    [findings, selectedIds],
  );
  const selectedIdList = useMemo(
    () => selectedFindings.map((finding) => finding.id),
    [selectedFindings],
  );
  const canPreview = Boolean(client && session && selectedIdList.length > 0);
  const selectedPreviewKey = `${session?.id ?? ""}:${reviewEvent}:${selectedIdList.join(",")}`;

  const buildPreview = useCallback(async () => {
    if (!client || !session) {
      return;
    }
    setPreviewState(loadingApiState());
    const state = await loadApiResource(() =>
      client.createGitHubPreview(session.id, {
        finding_ids: selectedIdList,
        review_event: reviewEvent,
      }),
    );
    setPreviewState(state);
  }, [client, reviewEvent, selectedIdList, session]);

  useEffect(() => {
    if (!canPreview) {
      previewAutoKeyRef.current = "";
      return;
    }
    if (previewAutoKeyRef.current === selectedPreviewKey) {
      return;
    }
    previewAutoKeyRef.current = selectedPreviewKey;
    queueMicrotask(() => void buildPreview());
  }, [buildPreview, canPreview, selectedPreviewKey]);

  async function buildCopyPacket(copyToClipboard: boolean) {
    if (!client || !session) {
      return;
    }
    setActionMessage("");
    setCopyPacketState(loadingApiState());
    const state = await loadApiResource(() =>
      client.createReviewCopyPacket(session.id, {
        finding_ids: selectedIdList,
        format: "markdown",
        include_evidence: true,
        include_counter_evidence: true,
        include_code_snippets: false,
        target_agent: "reviewer",
      }),
    );
    setCopyPacketState(state);
    if (state.status !== "success" || !copyToClipboard) {
      return;
    }
    if (!window.cocode?.writeClipboard) {
      setActionMessage("Clipboard bridge is unavailable.");
      return;
    }
    const copied = await loadApiResource(() =>
      window.cocode!.writeClipboard(state.data.content),
    );
    setActionMessage(
      copied.status === "success"
        ? "Copy packet copied to clipboard"
        : copied.status === "error"
          ? copied.error.message
          : "Copy failed",
    );
  }

  function toggleFinding(findingId: string) {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(findingId)) {
        next.delete(findingId);
      } else {
        next.add(findingId);
      }
      return next;
    });
  }

  if (!client) {
    return (
      <ErrorState
        title="Backend client unavailable"
        description="Publish preview and copy packets need an active backend connection."
      />
    );
  }

  if (!session) {
    return (
      <EmptyState
        title="No review selected"
        description="Select a review session before preparing publish output."
        icon={GitPullRequestIcon}
      />
    );
  }

  return (
    <div aria-label="Publish review preview" className="flex flex-col gap-4">
      <PaneHeader
        title="Publish review"
        description="Preview the GitHub review body, inline comments, and detailed copy packet before publishing."
        actions={
          <div className="flex items-center gap-2">
            <Button
              disabled={!canPreview || previewState.status === "loading"}
              size="sm"
              variant="outline"
              onClick={() => void buildPreview()}
            >
              <FileSearchIcon data-icon="inline-start" />
              Preview
            </Button>
            <Button
              disabled={!canPreview || copyPacketState.status === "loading"}
              size="sm"
              onClick={() => void buildCopyPacket(true)}
            >
              <CopyIcon data-icon="inline-start" />
              Copy packet
            </Button>
          </div>
        }
      />

      <div className="grid gap-4 2xl:grid-cols-[360px_minmax(0,1fr)]">
        <aside className="flex min-w-0 flex-col gap-4">
          <div className="rounded-lg border">
            <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
              <div>
                <div className="text-sm font-medium">Accepted findings</div>
                <div className="text-muted-foreground mt-1 text-xs">
                  {selectedIds.size} selected
                </div>
              </div>
              <Button
                disabled={findings.length === 0}
                size="sm"
                variant="outline"
                onClick={() =>
                  setSelectedIds(new Set(findings.map((finding) => finding.id)))
                }
              >
                All
              </Button>
            </div>
            {acceptedFindings.status === "loading" && (
              <LoadingRows rows={4} className="p-4" />
            )}
            {acceptedFindings.status === "error" && (
              <div className="p-4">
                <ErrorState
                  title="Accepted findings failed to load"
                  description={acceptedFindings.error.message}
                />
              </div>
            )}
            {acceptedFindings.status === "success" && findings.length === 0 && (
              <EmptyState
                title="No accepted findings yet"
                description="Accept findings before building a publish preview."
                icon={InboxIcon}
              />
            )}
            {findings.length > 0 && (
              <div className="flex flex-col">
                {findings.map((finding) => (
                  <label
                    key={finding.id}
                    className="hover:bg-surface flex cursor-pointer gap-3 border-b px-4 py-3 last:border-b-0"
                  >
                    <Checkbox
                      checked={selectedIds.has(finding.id)}
                      onCheckedChange={() => toggleFinding(finding.id)}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="line-clamp-2 text-sm font-medium">
                        {finding.canonical_claim}
                      </span>
                      <span className="text-muted-foreground mt-1 block truncate text-xs">
                        {formatFindingLocation(finding)}
                      </span>
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="rounded-lg border p-4">
            <label className="flex flex-col gap-2 text-sm font-medium">
              Review event
              <NativeSelect
                value={reviewEvent}
                onChange={(event) => setReviewEvent(event.target.value)}
              >
                <NativeSelectOption value="COMMENT">Comment</NativeSelectOption>
                <NativeSelectOption value="REQUEST_CHANGES">
                  Request changes
                </NativeSelectOption>
                <NativeSelectOption value="APPROVE">Approve</NativeSelectOption>
              </NativeSelect>
            </label>
            <div className="mt-3 rounded-md border p-3">
              <Badge variant="outline">GitHub submit unavailable</Badge>
              <p className="text-muted-foreground mt-2 text-xs leading-5">
                Preview and copy packet are ready; direct submit waits for the
                backend publish route.
              </p>
            </div>
            {actionMessage && (
              <p className="text-muted-foreground mt-2 text-sm">
                {actionMessage}
              </p>
            )}
          </div>
        </aside>

        <div className="grid min-w-0 gap-4 min-[1800px]:grid-cols-2">
          <GitHubPreviewPane state={previewState} />
          <CopyPacketPreviewPane
            state={copyPacketState}
            onBuild={() => void buildCopyPacket(false)}
            disabled={!canPreview || copyPacketState.status === "loading"}
          />
        </div>
      </div>
    </div>
  );
}

function GitHubPreviewPane({
  state,
}: {
  state: Loadable<GitHubPreviewResponse>;
}) {
  const [copyState, setCopyState] = useState<
    "idle" | "loading" | "copied" | "error"
  >("idle");
  const comments =
    state.status === "success" ? (state.data.comments ?? []) : [];
  const warnings =
    state.status === "success" ? (state.data.warnings ?? []) : [];

  async function copyReviewPreview() {
    if (state.status !== "success") {
      return;
    }
    setCopyState("loading");
    const copied = await loadApiResource(async () => {
      if (!window.cocode?.writeClipboard) {
        throw new Error("Clipboard bridge is unavailable");
      }
      await window.cocode.writeClipboard(
        formatGitHubPreviewClipboard(state.data),
      );
    });
    setCopyState(copied.status === "success" ? "copied" : "error");
  }

  return (
    <section className="overflow-hidden rounded-lg border bg-[#f6f8fa]">
      <div className="flex items-start justify-between gap-3 border-b bg-white px-4 py-3">
        <div>
          <div className="text-sm font-medium">GitHub review preview</div>
          <div className="text-muted-foreground mt-1 text-xs">
            Review body, inline comments, warnings, and checklist.
          </div>
        </div>
        <Button
          disabled={state.status !== "success" || copyState === "loading"}
          size="sm"
          variant="outline"
          onClick={() => void copyReviewPreview()}
        >
          <CopyIcon data-icon="inline-start" />
          {copyState === "copied" ? "Copied" : "Copy review"}
        </Button>
      </div>
      {state.status === "idle" && (
        <EmptyState
          title="No preview yet"
          description="Select accepted findings and build a preview."
          icon={FileSearchIcon}
        />
      )}
      {state.status === "loading" && <LoadingRows rows={5} className="p-4" />}
      {state.status === "error" && (
        <div className="p-4">
          <ErrorState
            title="Preview failed"
            description={state.error.message}
          />
        </div>
      )}
      {state.status === "success" && (
        <ScrollArea className="h-[620px]">
          <div className="flex flex-col gap-4 p-4">
            <GitHubPreviewChecklistView preview={state.data} />
            <GitHubReviewBodyPreview body={state.data.body} />
            <section>
              <div className="mb-2 flex items-center justify-between gap-2">
                <div className="text-sm font-medium">Inline comments</div>
                <Badge variant="outline">{comments.length}</Badge>
              </div>
              {comments.length === 0 ? (
                <EmptyState
                  className="border bg-white"
                  title="No inline comments"
                  description="This preview will publish as a summary review."
                  icon={MessageSquareIcon}
                />
              ) : (
                <div className="relative ml-3 flex flex-col gap-4 border-l-2 border-[#d0d7de] pl-5">
                  {comments.map((comment, index) => (
                    <GitHubInlineCommentPreview
                      key={`${comment.finding_id}:${comment.path ?? "summary"}:${comment.line ?? index}`}
                      comment={comment}
                      index={index}
                    />
                  ))}
                </div>
              )}
            </section>
            {warnings.length > 0 && (
              <section>
                <div className="mb-2 text-sm font-medium">Warnings</div>
                <div className="flex flex-col gap-2">
                  {warnings.map((warning) => (
                    <div
                      key={`${warning.finding_id}:${warning.message}`}
                      className="rounded-md border p-3"
                    >
                      <div className="text-sm font-medium">
                        {warning.path || warning.finding_id}
                      </div>
                      <p className="text-muted-foreground mt-1 text-sm">
                        {warning.message}
                      </p>
                    </div>
                  ))}
                </div>
              </section>
            )}
          </div>
        </ScrollArea>
      )}
    </section>
  );
}

function GitHubReviewBodyPreview({ body }: { body: string }) {
  return (
    <article className="overflow-hidden rounded-lg border border-[#d0d7de] bg-white shadow-[0_1px_2px_rgba(31,35,40,0.04)]">
      <div className="flex items-center justify-between gap-3 border-b border-[#d0d7de] bg-[#f6f8fa] px-4 py-3">
        <div className="min-w-0">
          <span className="font-semibold">cocode</span>
          <Badge className="ml-2" variant="outline">
            Bot
          </Badge>
          <span className="text-muted-foreground ml-2">left a review</span>
        </div>
        <span className="text-muted-foreground font-mono text-lg leading-none">
          ...
        </span>
      </div>
      <div className="px-4 py-4">
        <div className="mb-2 text-xs font-medium">Review body</div>
        <MarkdownMessage className="text-sm leading-6" content={body} />
      </div>
    </article>
  );
}

function GitHubInlineCommentPreview({
  comment,
  index,
}: {
  comment: GitHubPreviewResponse["comments"][number];
  index: number;
}) {
  return (
    <article className="relative overflow-hidden rounded-lg border border-[#d0d7de] bg-white shadow-[0_1px_2px_rgba(31,35,40,0.04)] before:absolute before:top-5 before:-left-[27px] before:h-2 before:w-2 before:rounded-full before:border-2 before:border-[#d0d7de] before:bg-white">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[#d0d7de] bg-[#f6f8fa] px-3 py-2 text-sm">
        <div className="min-w-0">
          <span className="font-mono text-xs font-semibold">
            {formatGitHubCommentLocation(comment)}
          </span>
          {comment.line ? (
            <span className="text-muted-foreground ml-2 text-xs">
              comment on line {comment.line}
            </span>
          ) : null}
        </div>
        <Badge variant={comment.unanchored ? "destructive" : "outline"}>
          {comment.unanchored ? "unanchored" : "anchored"}
        </Badge>
      </div>
      <div className="px-4 py-3">
        <div className="mb-2 flex items-center gap-2 text-xs">
          <span className="font-semibold">cocode</span>
          <Badge variant="outline">Bot</Badge>
          <span className="text-muted-foreground">
            inline draft {index + 1}
          </span>
        </div>
        <MarkdownMessage className="text-sm leading-6" content={comment.body} />
        {comment.warning && (
          <p className="text-destructive mt-3 text-xs leading-5 [overflow-wrap:anywhere] break-words">
            {comment.warning}
          </p>
        )}
      </div>
    </article>
  );
}

function formatGitHubCommentLocation(
  comment: GitHubPreviewResponse["comments"][number],
) {
  if (!comment.path) {
    return "Summary comment";
  }
  if (comment.line) {
    return `${comment.path}:L${comment.line}`;
  }
  if (comment.position) {
    return `${comment.path}:position ${comment.position}`;
  }
  return comment.path;
}

function formatGitHubPreviewClipboard(preview: GitHubPreviewResponse) {
  const lines = [
    "GitHub review body",
    "",
    preview.body.trim(),
    "",
    "Inline comments",
    "",
  ];
  if (preview.comments.length === 0) {
    lines.push("No inline comments.", "");
  } else {
    preview.comments.forEach((comment, index) => {
      lines.push(`${index + 1}. ${formatGitHubCommentLocation(comment)}`);
      lines.push(
        comment.unanchored ? "Status: unanchored" : "Status: anchored",
      );
      if (comment.warning) {
        lines.push(`Warning: ${comment.warning}`);
      }
      lines.push("", comment.body.trim(), "");
    });
  }
  if (preview.warnings.length > 0) {
    lines.push("Warnings", "");
    preview.warnings.forEach((warning, index) => {
      lines.push(
        `${index + 1}. ${warning.path || warning.finding_id}: ${warning.message}`,
      );
    });
  }
  return `${lines
    .join("\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim()}\n`;
}

function GitHubPreviewChecklistView({
  preview,
}: {
  preview: GitHubPreviewResponse;
}) {
  const items = [
    ["Selected findings", preview.checklist.has_selected_findings],
    ["Inline comments", preview.checklist.has_inline_comments],
    ["Unanchored comments", preview.checklist.has_unanchored_comments],
    ["Can publish inline", preview.checklist.can_publish_inline],
    ["Can publish summary-only", preview.checklist.can_publish_summary_only],
  ] as const;
  return (
    <div className="grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-2">
      {items.map(([label, ok]) => (
        <div
          key={label}
          className="flex items-center gap-2 rounded-md border p-2"
        >
          <Badge variant={ok ? "secondary" : "outline"}>
            {ok ? "yes" : "no"}
          </Badge>
          <span className="text-sm">{label}</span>
        </div>
      ))}
    </div>
  );
}

function CopyPacketPreviewPane({
  disabled,
  onBuild,
  state,
}: {
  disabled: boolean;
  onBuild: () => void;
  state: Loadable<CreateCopyPacketResponse>;
}) {
  return (
    <section className="rounded-lg border">
      <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
        <div>
          <div className="text-sm font-medium">Copy packet</div>
          <div className="text-muted-foreground mt-1 text-xs">
            Markdown packet for handing findings to another agent.
          </div>
        </div>
        <Button
          disabled={disabled}
          size="sm"
          variant="outline"
          onClick={onBuild}
        >
          Build
        </Button>
      </div>
      {state.status === "idle" && (
        <EmptyState
          title="No packet yet"
          description="Build or copy a packet from the selected findings."
          icon={CopyIcon}
        />
      )}
      {state.status === "loading" && <LoadingRows rows={5} className="p-4" />}
      {state.status === "error" && (
        <div className="p-4">
          <ErrorState
            title="Copy packet failed"
            description={state.error.message}
          />
        </div>
      )}
      {state.status === "success" && (
        <ScrollArea className="h-[620px]">
          <div className="flex flex-col gap-3 p-4">
            <div className="flex flex-wrap gap-2">
              <Badge variant="secondary">
                {state.data.finding_count} findings
              </Badge>
              <Badge variant="outline">
                {state.data.token_estimate} tokens
              </Badge>
              <Badge variant="outline">{state.data.format}</Badge>
            </div>
            <pre className="bg-surface overflow-x-auto rounded-md border p-3 text-xs leading-5 whitespace-pre-wrap">
              {state.data.content}
            </pre>
          </div>
        </ScrollArea>
      )}
    </section>
  );
}
