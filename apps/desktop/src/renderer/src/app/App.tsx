import { useEffect, useMemo, useState } from "react";
import {
  ArrowUpIcon,
  BellIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleIcon,
  ClockIcon,
  Code2Icon,
  CopyIcon,
  FileSearchIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  InboxIcon,
  MessageSquareIcon,
  MoreHorizontalIcon,
  PanelRightIcon,
  PauseIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  ShieldCheckIcon,
  SparklesIcon,
  SquareIcon,
  TerminalIcon,
} from "lucide-react";

import {
  AppShell,
  EmptyState,
  ErrorState,
  LoadingRows,
  PaneHeader,
  SearchCommandDialog,
  SidebarNavButton,
  SidebarSection,
  TooltipIconButton,
  type SearchCommandGroup,
} from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  InputGroup,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  createCocodeClient,
  errorApiState,
  loadApiResource,
  loadingApiState,
  type ApiSessionResponse,
  type Loadable,
} from "@/lib/api";
import { cn } from "@/lib/utils";

const threads = [
  { title: "Review and triage issues", age: "1w", active: true },
  { title: "Redesign app modern UI", age: "1w", active: false },
  { title: "Create flow diagram", age: "2w", active: false },
  { title: "Add icons", age: "1mo", active: false },
];

const projects = [
  "pharmakon-polymarket-trading",
  "zap-earn-service",
  "kd-market-service",
  "cocode",
];

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

export function App() {
  const [backendStatus, setBackendStatus] = useState("loading");
  const [backendUrl, setBackendUrl] = useState("");
  const [apiSession, setApiSession] =
    useState<Loadable<ApiSessionResponse>>(loadingApiState);
  const [searchOpen, setSearchOpen] = useState(false);

  useEffect(() => {
    let canceled = false;

    const bridge = window.cocode;
    if (!bridge) {
      queueMicrotask(() => {
        if (!canceled) {
          setBackendStatus("unavailable");
          setApiSession(
            errorApiState(new Error("Desktop bridge is unavailable")),
          );
        }
      });
      return () => {
        canceled = true;
      };
    }

    void bridge
      .getBackendInfo()
      .then((info) => {
        if (canceled) {
          return;
        }
        setBackendStatus(info.status);
        setBackendUrl(info.baseUrl);

        const client = createCocodeClient(info);
        void loadApiResource(() => client.session()).then((state) => {
          if (canceled) {
            return;
          }
          setApiSession(state);
          if (state.status === "error") {
            setBackendStatus("unavailable");
          }
        });
      })
      .catch(() => {
        if (!canceled) {
          setApiSession(
            errorApiState(new Error("Backend info is unavailable")),
          );
          setBackendStatus("unavailable");
        }
      });

    return () => {
      canceled = true;
    };
  }, []);

  const backendDetail =
    apiSession.status === "error"
      ? apiSession.error.message
      : backendUrl || "Waiting for backend info";
  const searchGroups = useMemo<SearchCommandGroup[]>(
    () => [
      {
        heading: "Reviews",
        commands: [
          {
            title: "Open active billing review",
            description: "PR #482 billing auth guard",
            shortcut: "Enter",
            icon: GitPullRequestIcon,
          },
          {
            title: "Create new review thread",
            description: "Start from PR URL, local changes, or branch compare",
            shortcut: "N",
            icon: PlusIcon,
          },
        ],
      },
      {
        heading: "Actions",
        commands: [
          {
            title: "Configure CLI agents",
            description: "Codex, Gemini, OpenCode, and custom CLIs",
            icon: TerminalIcon,
          },
          {
            title: "Open app settings",
            description: "Credentials, presets, privacy, and logs",
            icon: SettingsIcon,
          },
        ],
      },
    ],
    [],
  );

  return (
    <>
      <AppShell
        sidebar={
          <Sidebar
            backendStatus={backendStatus}
            onOpenSearch={() => setSearchOpen(true)}
          />
        }
        header={<TopNav onOpenSearch={() => setSearchOpen(true)} />}
        detailPane={<ReviewPane />}
      >
        <ReviewThread apiSession={apiSession} backendDetail={backendDetail} />
      </AppShell>
      {searchOpen && (
        <SearchCommandDialog
          open={searchOpen}
          onOpenChange={setSearchOpen}
          groups={searchGroups}
        />
      )}
    </>
  );
}

function Sidebar({
  backendStatus,
  onOpenSearch,
}: {
  backendStatus: string;
  onOpenSearch: () => void;
}) {
  return (
    <>
      <div className="flex h-12 items-center gap-2 px-4">
        <div className="bg-destructive/80 size-3 rounded-full" />
        <div className="bg-warning/80 size-3 rounded-full" />
        <div className="bg-success/80 size-3 rounded-full" />
      </div>

      <div className="flex items-center gap-2 px-4 pb-4">
        <div className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg">
          <BotIcon />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold">cocode</p>
          <p className="text-sidebar-muted truncate text-xs">
            Local review cockpit
          </p>
        </div>
      </div>

      <nav className="flex flex-col gap-1 px-2">
        <SidebarNavButton icon={PlusIcon} label="New thread" />
        <SidebarNavButton
          icon={SearchIcon}
          label="Search"
          onClick={onOpenSearch}
        />
        <SidebarNavButton icon={SparklesIcon} label="Plugins" />
        <SidebarNavButton icon={ClockIcon} label="Automations" />
      </nav>

      <SidebarSection
        title="Pinned"
        action={<SquareIcon className="opacity-70" />}
      >
        {threads.map((thread) => (
          <SidebarNavButton
            key={thread.title}
            label={thread.title}
            meta={thread.age}
            active={thread.active}
          />
        ))}
      </SidebarSection>

      <SidebarSection
        title="Projects"
        action={<SquareIcon className="opacity-70" />}
      >
        {projects.map((project) => (
          <SidebarNavButton
            key={project}
            icon={GitBranchIcon}
            label={project}
          />
        ))}
      </SidebarSection>

      <div className="mt-auto flex flex-col gap-1 p-2">
        <Separator className="-mx-2 mb-1" />
        <SidebarNavButton icon={SettingsIcon} label="Settings" />
        <div className="text-sidebar-muted px-2 pt-1 pb-2 text-xs">
          Backend {backendStatus}
        </div>
      </div>
    </>
  );
}

function TopNav({ onOpenSearch }: { onOpenSearch: () => void }) {
  return (
    <PaneHeader
      icon={GitPullRequestIcon}
      title="PR #482 billing auth guard"
      description="pharmakon-polymarket"
      actions={
        <>
          <Button size="sm" variant="outline" onClick={onOpenSearch}>
            <SearchIcon data-icon="inline-start" />
            Search
          </Button>
          <Button size="sm" variant="outline">
            <PanelRightIcon data-icon="inline-start" />
            Ask all agents
          </Button>
          <CommitDropdown />
          <Badge variant="secondary">+938 -664</Badge>
          <TooltipIconButton
            label="Notifications"
            size="icon-sm"
            variant="ghost"
          >
            <BellIcon />
          </TooltipIconButton>
          <TooltipIconButton
            label="More actions"
            size="icon-sm"
            variant="ghost"
          >
            <MoreHorizontalIcon />
          </TooltipIconButton>
        </>
      }
    />
  );
}

function CommitDropdown() {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline">
          Commit
          <ChevronDownIcon data-icon="inline-end" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel>Review actions</DropdownMenuLabel>
        <DropdownMenuGroup>
          <DropdownMenuItem>Copy selected packet</DropdownMenuItem>
          <DropdownMenuItem>Create draft review</DropdownMenuItem>
          <DropdownMenuItem>Open GitHub preview</DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive">Cancel review</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ReviewThread({
  apiSession,
  backendDetail,
}: {
  apiSession: Loadable<ApiSessionResponse>;
  backendDetail: string;
}) {
  return (
    <section className="flex min-w-0 flex-col">
      <ScrollArea className="flex-1 px-6 py-5">
        <div className="mx-auto flex max-w-3xl flex-col gap-5">
          {apiSession.status === "loading" && (
            <LoadingRows rows={2} className="rounded-lg border p-4" />
          )}
          {apiSession.status === "error" && (
            <ErrorState
              title="Backend connection failed"
              description={apiSession.error.message}
            />
          )}

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
                <span className="text-muted-foreground">Phase 1 of 3</span>
              </div>
              <p className="text-sm leading-6">
                I found a likely authorization bypass in the billing route
                group. Codex, Gemini, OpenCode, and Local Verifier agree on the
                affected line range and there is supporting evidence from route
                setup, middleware, and tests.
              </p>
            </div>
          </div>

          <ChangedFilesPanel />
          <FindingsPanel />
        </div>
      </ScrollArea>

      <MessageComposer backendDetail={backendDetail} />
    </section>
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

function MessageComposer({ backendDetail }: { backendDetail: string }) {
  return (
    <div className="bg-surface-raised border-t p-4">
      <div className="bg-background mx-auto max-w-3xl rounded-2xl border shadow-sm">
        <InputGroup className="min-h-24 items-stretch border-0">
          <InputGroupTextarea
            aria-label="Follow-up prompt"
            placeholder="Ask a follow-up grounded in this evidence bundle..."
            className="min-h-20"
          />
        </InputGroup>
        <div className="flex items-center justify-between border-t px-3 py-2">
          <div className="flex items-center gap-2">
            <Button size="sm" variant="ghost">
              <MessageSquareIcon data-icon="inline-start" />
              Review
            </Button>
            <ComposerDropdown label="GPT-5.5 Fast" />
            <ComposerDropdown label="Low" />
          </div>
          <InputGroupButton size="icon-sm" aria-label="Send follow-up">
            <ArrowUpIcon />
          </InputGroupButton>
        </div>
      </div>
      <div className="text-muted-foreground mx-auto mt-2 max-w-3xl truncate text-center text-xs">
        {backendDetail}
      </div>
    </div>
  );
}

function ComposerDropdown({ label }: { label: string }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="ghost">
          {label}
          <ChevronDownIcon data-icon="inline-end" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuGroup>
          <DropdownMenuItem>Fast</DropdownMenuItem>
          <DropdownMenuItem>Balanced</DropdownMenuItem>
          <DropdownMenuItem>Deep</DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ReviewPane() {
  return (
    <aside className="bg-surface-raised min-w-0 border-l">
      <div className="flex h-full flex-col">
        <PaneHeader
          icon={Code2Icon}
          title="Review"
          actions={
            <TooltipIconButton
              label="Pause review"
              size="icon-sm"
              variant="ghost"
            >
              <PauseIcon />
            </TooltipIconButton>
          }
        />

        <Tabs defaultValue="diff" className="min-h-0 flex-1 gap-0">
          <div className="flex items-center justify-between gap-2 border-b px-4 py-2">
            <TabsList variant="line">
              <TabsTrigger value="diff">Diff</TabsTrigger>
              <TabsTrigger value="evidence">Evidence</TabsTrigger>
              <TabsTrigger value="publish">Publish</TabsTrigger>
            </TabsList>
            <div className="flex min-w-0 items-center gap-2 text-xs">
              <Badge variant="outline">Unstaged</Badge>
              <Badge variant="secondary">4</Badge>
              <span className="text-muted-foreground truncate">billing.go</span>
            </div>
          </div>

          <TabsContent value="diff" className="min-h-0">
            <ScrollArea className="h-full font-mono text-xs">
              <CodeLine
                num={2}
                text="@@ RegisterBillingRoutes @@"
                tone="context"
              />
              <CodeLine
                num={3}
                text="func RegisterBillingRoutes(r *mux.Router) {"
              />
              <CodeLine
                num={4}
                text={'  r.HandleFunc("/billing/invoices", listInvoices)'}
                tone="removed"
              />
              <CodeLine
                num={5}
                text={'  protected := r.PathPrefix("/api").Subrouter()'}
                tone="added"
              />
              <CodeLine
                num={6}
                text="  protected.Use(middleware.RequireAuth())"
                tone="added"
              />
              <CodeLine
                num={7}
                text={
                  '  protected.HandleFunc("/billing/invoices", listInvoices)'
                }
                tone="added"
              />
              <CodeLine
                num={8}
                text={
                  '  protected.HandleFunc("/billing/payouts", createPayout)'
                }
                tone="added"
              />
              <CodeLine num={9} text="}" />
              <CodeLine num={10} text="" />
              <CodeLine
                num={11}
                text="@@ handlers/payouts.go @@"
                tone="context"
              />
              <CodeLine
                num={12}
                text="func createPayout(w http.ResponseWriter, r *http.Request) {"
              />
              <CodeLine
                num={13}
                text={'  user := r.Context().Value("user").(*User)'}
              />
              <CodeLine num={14} text="  // payout logic..." />
              <CodeLine num={15} text="}" />
            </ScrollArea>
          </TabsContent>

          <TabsContent value="evidence" className="min-h-0">
            <div className="p-4">
              <ErrorState
                title="Counter-evidence incomplete"
                description="The verifier found route and middleware evidence, but no matching regression test yet."
              />
            </div>
          </TabsContent>

          <TabsContent value="publish" className="min-h-0">
            <div className="p-4">
              <EmptyState
                title="Nothing accepted yet"
                description="Accepted findings will appear here for copy packets and GitHub preview."
                icon={CopyIcon}
              />
            </div>
          </TabsContent>
        </Tabs>

        <div className="border-t p-4">
          <div className="bg-background rounded-lg border p-3">
            <div className="mb-3 flex items-center gap-2">
              <Badge variant="destructive">High</Badge>
              <Badge variant="secondary">Verified</Badge>
            </div>
            <p className="text-sm font-medium">
              Auth middleware skipped on billing route
            </p>
            <p className="text-muted-foreground mt-2 text-sm">
              Billing endpoints are reachable without RequireAuth on the route
              group.
            </p>
            <div className="mt-3 flex gap-2">
              <Button size="sm">
                <CheckIcon data-icon="inline-start" />
                Accept
              </Button>
              <Button size="sm" variant="outline">
                <CopyIcon data-icon="inline-start" />
                Copy
              </Button>
            </div>
          </div>
        </div>
      </div>
    </aside>
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
          variant={finding.severity === "High" ? "destructive" : "secondary"}
        >
          {finding.severity}
        </Badge>
        <Badge variant="outline">{finding.status}</Badge>
      </div>
    </button>
  );
}

function CodeLine({
  num,
  text,
  tone = "default",
}: {
  num: number;
  text: string;
  tone?: "default" | "added" | "removed" | "context";
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-[48px_minmax(0,1fr)] border-b border-transparent leading-6",
        tone === "added" && "bg-code-added",
        tone === "removed" && "bg-code-removed",
        tone === "context" && "bg-surface",
      )}
    >
      <span className="text-muted-foreground pr-3 text-right select-none">
        {num}
      </span>
      <span className="truncate px-3 whitespace-pre">{text || " "}</span>
    </div>
  );
}
