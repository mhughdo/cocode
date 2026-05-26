import { useCallback, useEffect, useState } from "react";
import {
  BotIcon,
  CircleIcon,
  FileSearchIcon,
  InboxIcon,
  MapIcon,
  MessageSquareIcon,
  PauseIcon,
  ShieldCheckIcon,
} from "lucide-react";

import { EmptyState } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  type AgentConfig,
  type ApiClient,
  errorApiState,
  type Finding,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type Repository,
  type ReviewSession,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { CentralizedChatScreen } from "../chat/centralized-chat-screen";
import { EvidenceMapScreen } from "../evidence/evidence-map-screen";
import { FindingDetailScreen } from "../findings/finding-detail-screen";
import { FindingFollowUpScreen } from "../findings/finding-follow-up-screen";
import { PublishReviewScreen } from "./publish-review-screen";
import { ReviewDetailsScreen } from "./review-details-screen";
import { ReviewFindingsBoard } from "./review-findings-board";
import { useReviewSessionLiveData } from "./review-live-data";

const REVIEW_THREAD_TAB_CLASS =
  "mt-4 flex min-h-0 flex-1 flex-col overflow-y-auto pr-1 pl-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden";

const changedFiles = [
  { path: "api/routes/billing.go", additions: 132, deletions: 18 },
  { path: "middleware/auth.go", additions: 89, deletions: 4 },
  { path: "handlers/payouts.go", additions: 64, deletions: 7 },
  { path: "tests/billing_routes_test.go", additions: 25, deletions: 1 },
];

const findings = [
  {
    title: "Auth middleware skipped on billing route",
    file: "api/routes/billing.go",
    lines: "L132-L135",
    severity: "High",
    status: "Verified",
  },
  {
    title: "Webhook payload not validated",
    file: "api/webhooks/stripe.go",
    lines: "L78-L92",
    severity: "Medium",
    status: "Needs triage",
  },
  {
    title: "Admin export route lacks role check",
    file: "api/routes/admin.go",
    lines: "L41-L48",
    severity: "High",
    status: "Needs triage",
  },
];

export function ReviewThread({
  activeRepository,
  agentConfigs,
  client,
  session,
}: {
  activeRepository?: Repository;
  agentConfigs: Loadable<AgentConfig[]>;
  client: ApiClient | null;
  session?: ReviewSession;
}) {
  const [activeTab, setActiveTab] = useState("chat");
  const [evidenceMapFinding, setEvidenceMapFinding] = useState<Finding | null>(
    null,
  );
  const [followUpFinding, setFollowUpFinding] = useState<Finding | null>(null);
  const [detailFinding, setDetailFinding] = useState<Finding | null>(null);
  const live = useReviewSessionLiveData(client, session);

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setEvidenceMapFinding(null);
        setFollowUpFinding(null);
        setDetailFinding(null);
        setActiveTab("chat");
      }
    });
    return () => {
      canceled = true;
    };
  }, [session?.id]);

  const openEvidenceMap = useCallback((finding: Finding) => {
    setEvidenceMapFinding(finding);
    setActiveTab("evidence-map");
  }, []);

  const openFollowUp = useCallback((finding: Finding) => {
    setFollowUpFinding(finding);
    setActiveTab("follow-up");
  }, []);

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-hidden px-6 py-5">
        <div className="mx-auto flex h-full min-h-0 w-full max-w-[1500px] flex-col gap-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <h1 className="truncate text-2xl font-semibold tracking-tight">
                  {session?.title ?? "Review thread"}
                </h1>
                {live.refreshState.status === "refreshing" && (
                  <Badge variant="outline">Refreshing</Badge>
                )}
                {live.refreshState.status === "error" && (
                  <Badge variant="outline">Refresh issue</Badge>
                )}
              </div>
              <p className="text-muted-foreground mt-1 text-sm">
                {session
                  ? `${session.review_depth} review • ${session.status}`
                  : "Create or select a review session to stream live progress."}
              </p>
              {live.refreshState.status === "error" && (
                <p className="text-destructive mt-1 max-w-2xl truncate text-xs">
                  {live.refreshState.message}
                </p>
              )}
            </div>
            {session && (
              <ReviewControlButtons
                client={client}
                onSessionUpdated={live.setSession}
                session={live.session ?? session}
              />
            )}
          </div>

          <Tabs
            value={activeTab}
            onValueChange={setActiveTab}
            className="min-h-0 flex-1"
          >
            <TabsList
              variant="line"
              className="border-border h-9 w-full justify-start gap-8 border-b p-0"
            >
              <TabsTrigger
                value="chat"
                className="h-9 flex-none rounded-none border-0 px-0 text-[13px]"
              >
                Chat
              </TabsTrigger>
              <TabsTrigger
                value="findings"
                className="h-9 flex-none rounded-none border-0 px-0 text-[13px]"
              >
                Findings
              </TabsTrigger>
              <TabsTrigger
                value="publish"
                className="h-9 flex-none rounded-none border-0 px-0 text-[13px]"
              >
                Publish
              </TabsTrigger>
              <TabsTrigger value="details" className="hidden">
                Details
              </TabsTrigger>
              <TabsTrigger value="finding-detail" className="hidden">
                Finding detail
              </TabsTrigger>
              <TabsTrigger value="evidence-map" className="hidden">
                Evidence map
              </TabsTrigger>
              <TabsTrigger value="follow-up" className="hidden">
                Follow-up
              </TabsTrigger>
            </TabsList>

            <TabsContent
              value="chat"
              className={cn(REVIEW_THREAD_TAB_CLASS, "overflow-hidden")}
            >
              {session ? (
                <CentralizedChatScreen
                  agentConfigs={agentConfigs}
                  client={client}
                  events={live.events}
                  findings={live.findings}
                  onOpenFindings={() => setActiveTab("findings")}
                  session={live.session ?? session}
                  summary={live.summary}
                />
              ) : (
                <>
                  <div className="bg-surface self-end rounded-full px-4 py-2 text-sm">
                    Review this PR for auth, billing, and data integrity.
                  </div>
                  <div className="flex items-start gap-3">
                    <div className="bg-primary text-primary-foreground mt-1 flex size-7 items-center justify-center rounded-md">
                      <BotIcon />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="mb-2 flex items-center gap-2 text-sm">
                        <span className="font-medium">cocode</span>
                        <Badge variant="secondary">4 agents</Badge>
                        <span className="text-muted-foreground">
                          Phase 1 of 3
                        </span>
                      </div>
                      <p className="text-sm leading-6">
                        I found a likely authorization bypass in the billing
                        route group. Codex, Gemini, OpenCode, and Local Verifier
                        agree on the affected line range and there is supporting
                        evidence from route setup, middleware, and tests.
                      </p>
                    </div>
                  </div>
                  <ChangedFilesPanel />
                  <FindingsPanel />
                </>
              )}
            </TabsContent>

            <TabsContent value="details" className={REVIEW_THREAD_TAB_CLASS}>
              <ReviewDetailsScreen
                agentConfigs={agentConfigs}
                client={client}
                events={live.events}
                session={live.session ?? session}
                streamState={live.streamState}
              />
            </TabsContent>

            <TabsContent value="findings" className={REVIEW_THREAD_TAB_CLASS}>
              <ReviewFindingsBoard
                client={client}
                findings={live.findings}
                onOpenDetail={(finding) => {
                  setDetailFinding(finding);
                  setActiveTab("finding-detail");
                }}
                onOpenEvidenceMap={openEvidenceMap}
                onOpenFollowUp={openFollowUp}
                session={live.session ?? session}
              />
            </TabsContent>

            <TabsContent
              value="finding-detail"
              className={REVIEW_THREAD_TAB_CLASS}
            >
              {detailFinding ? (
                <FindingDetailScreen
                  agentConfigs={agentConfigs}
                  client={client}
                  events={live.events}
                  finding={detailFinding}
                  onBack={() => setActiveTab("findings")}
                  onOpenEvidenceMap={openEvidenceMap}
                  onOpenFollowUp={openFollowUp}
                />
              ) : (
                <EmptyState
                  title="Select a finding first"
                  description="Open full detail from a selected finding to inspect code, evidence, and discussion."
                  icon={FileSearchIcon}
                />
              )}
            </TabsContent>

            <TabsContent
              value="evidence-map"
              className={REVIEW_THREAD_TAB_CLASS}
            >
              {evidenceMapFinding ? (
                <EvidenceMapScreen
                  activeRepository={activeRepository}
                  client={client}
                  finding={evidenceMapFinding}
                  onBack={() => setActiveTab("findings")}
                  onOpenFindingDetail={(finding) => {
                    setDetailFinding(finding);
                    setActiveTab("finding-detail");
                  }}
                />
              ) : (
                <EmptyState
                  title="Select a finding first"
                  description="Open Evidence Map from a selected finding to inspect graph context."
                  icon={MapIcon}
                />
              )}
            </TabsContent>

            <TabsContent value="follow-up" className={REVIEW_THREAD_TAB_CLASS}>
              {followUpFinding ? (
                <FindingFollowUpScreen
                  agentConfigs={agentConfigs}
                  client={client}
                  events={live.events}
                  finding={followUpFinding}
                  onBack={() => setActiveTab("findings")}
                />
              ) : (
                <EmptyState
                  title="Select a finding first"
                  description="Open Follow-up from a selected finding to ask scoped questions."
                  icon={MessageSquareIcon}
                />
              )}
            </TabsContent>

            <TabsContent value="publish" className={REVIEW_THREAD_TAB_CLASS}>
              <PublishReviewScreen
                client={client}
                session={live.session ?? session}
              />
            </TabsContent>
          </Tabs>
        </div>
      </div>
    </section>
  );
}

function ReviewControlButtons({
  client,
  onSessionUpdated,
  session,
}: {
  client: ApiClient | null;
  onSessionUpdated: (session: ReviewSession) => void;
  session: ReviewSession;
}) {
  const [controlState, setControlState] =
    useState<Loadable<ReviewSession>>(idleApiState());
  const isPaused = session.status === "paused";
  const isTerminal = ["completed", "failed", "canceled"].includes(
    session.status,
  );

  async function runControl(action: "pause" | "resume" | "cancel") {
    if (!client) {
      setControlState(
        errorApiState(new Error("Backend client is unavailable")),
      );
      return;
    }
    setControlState(loadingApiState());
    const state = await loadApiResource(() => {
      if (action === "pause") {
        return client.pauseReviewSession(session.id);
      }
      if (action === "resume") {
        return client.resumeReviewSession(session.id);
      }
      return client.cancelReviewSession(session.id);
    });
    setControlState(state);
    if (state.status === "success") {
      onSessionUpdated(state.data);
    }
  }

  return (
    <div className="flex shrink-0 items-center gap-2">
      {controlState.status === "error" && (
        <span className="text-destructive max-w-56 truncate text-xs">
          {controlState.error.message}
        </span>
      )}
      <Button
        disabled={isTerminal || controlState.status === "loading"}
        size="sm"
        variant="outline"
        onClick={() => void runControl(isPaused ? "resume" : "pause")}
      >
        <PauseIcon data-icon="inline-start" />
        {isPaused ? "Resume" : "Pause"}
      </Button>
      <Button
        disabled={isTerminal || controlState.status === "loading"}
        size="sm"
        variant="outline"
        onClick={() => void runControl("cancel")}
      >
        Cancel
      </Button>
    </div>
  );
}

function ChangedFilesPanel() {
  if (changedFiles.length === 0) {
    return (
      <EmptyState
        title="No changed files"
        description="Create or select a snapshot to review the diff."
        icon={InboxIcon}
      />
    );
  }

  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
        <span className="font-medium">4 files changed</span>
        <Button size="sm" variant="ghost">
          Undo
        </Button>
      </div>
      {changedFiles.map((file) => (
        <div
          key={file.path}
          className="flex items-center justify-between gap-3 border-b px-3 py-2 text-sm last:border-b-0"
        >
          <span className="truncate font-mono text-xs">{file.path}</span>
          <span className="flex shrink-0 items-center gap-2 text-xs">
            <span className="text-success">+{file.additions}</span>
            <span className="text-destructive">-{file.deletions}</span>
          </span>
        </div>
      ))}
    </section>
  );
}

function FindingsPanel() {
  return (
    <section className="bg-surface-raised rounded-lg border">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <div className="flex items-center gap-2">
          <ShieldCheckIcon />
          <span className="text-sm font-medium">Evidence-backed findings</span>
        </div>
        <Badge variant="secondary">18 total</Badge>
      </div>
      {findings.length === 0 ? (
        <EmptyState
          className="border-0"
          title="No findings yet"
          description="Findings will appear as agents and the local verifier produce evidence."
          icon={FileSearchIcon}
        />
      ) : (
        findings.map((finding) => (
          <FindingRow key={finding.title} finding={finding} />
        ))
      )}
    </section>
  );
}

function FindingRow({
  finding,
}: {
  finding: {
    title: string;
    file: string;
    lines: string;
    severity: string;
    status: string;
  };
}) {
  return (
    <button
      className="hover:bg-surface flex w-full items-start gap-3 border-b px-3 py-3 text-left last:border-b-0"
      type="button"
    >
      <CircleIcon className="text-destructive mt-1" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{finding.title}</div>
        <div className="text-muted-foreground mt-1 flex items-center gap-2 text-xs">
          <span className="truncate font-mono">{finding.file}</span>
          <span>{finding.lines}</span>
        </div>
      </div>
      <div className="flex shrink-0 gap-1">
        <Badge
          variant={
            finding.severity === "High"
              ? "severity-high"
              : finding.severity === "Medium"
                ? "severity-medium"
                : "severity-low"
          }
        >
          {finding.severity}
        </Badge>
        <Badge variant="outline">{finding.status}</Badge>
      </div>
    </button>
  );
}
