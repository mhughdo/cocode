import { useEffect, useState, type ReactNode } from "react";
import {
  ArrowUpIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleIcon,
  ClockIcon,
  Code2Icon,
  CopyIcon,
  GitBranchIcon,
  GitPullRequestIcon,
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
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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

  useEffect(() => {
    let canceled = false;
    void window.cocode
      ?.getBackendInfo()
      .then((info) => {
        if (!canceled) {
          setBackendStatus(info.status);
          setBackendUrl(info.baseUrl);
        }
      })
      .catch(() => {
        if (!canceled) {
          setBackendStatus("unavailable");
        }
      });

    return () => {
      canceled = true;
    };
  }, []);

  return (
    <main className="bg-background text-foreground flex min-h-screen">
      <aside className="bg-sidebar text-sidebar-foreground flex w-[244px] shrink-0 flex-col">
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

        <nav className="flex flex-col gap-1 px-2 text-sm">
          {[
            { label: "New chat", icon: PlusIcon },
            { label: "Search", icon: SearchIcon },
            { label: "Plugins", icon: SparklesIcon },
            { label: "Automations", icon: ClockIcon },
          ].map((item) => (
            <button
              key={item.label}
              className="text-sidebar-foreground/85 hover:bg-background/45 flex h-8 items-center gap-2 rounded-md px-2 text-left"
              type="button"
            >
              <item.icon />
              <span className="truncate">{item.label}</span>
            </button>
          ))}
        </nav>

        <SidebarSection title="Pinned">
          {threads.map((thread) => (
            <button
              key={thread.title}
              className={cn(
                "hover:bg-background/45 flex h-8 items-center justify-between gap-2 rounded-md px-2 text-left text-sm",
                thread.active && "bg-background/60",
              )}
              type="button"
            >
              <span className="truncate">{thread.title}</span>
              <span className="text-sidebar-muted text-xs">{thread.age}</span>
            </button>
          ))}
        </SidebarSection>

        <SidebarSection title="Projects">
          {projects.map((project) => (
            <button
              key={project}
              className="hover:bg-background/45 flex h-8 items-center gap-2 rounded-md px-2 text-left text-sm"
              type="button"
            >
              <GitBranchIcon />
              <span className="truncate">{project}</span>
            </button>
          ))}
        </SidebarSection>

        <div className="mt-auto flex flex-col gap-1 p-2">
          <button
            className="hover:bg-background/45 flex h-8 items-center gap-2 rounded-md px-2 text-left text-sm"
            type="button"
          >
            <SettingsIcon />
            <span>Settings</span>
          </button>
          <div className="text-sidebar-muted px-2 pt-1 pb-2 text-xs">
            Backend {backendStatus}
          </div>
        </div>
      </aside>

      <section className="bg-background flex min-w-0 flex-1 flex-col">
        <header className="bg-surface-raised flex h-12 items-center justify-between border-b px-4">
          <div className="flex min-w-0 items-center gap-3 text-sm">
            <GitPullRequestIcon />
            <span className="truncate font-medium">
              PR #482 billing auth guard
            </span>
            <span className="text-muted-foreground">pharmakon-polymarket</span>
          </div>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="outline">
              <PanelRightIcon data-icon="inline-start" />
              Review
            </Button>
            <Button size="sm" variant="outline">
              Commit
              <ChevronDownIcon data-icon="inline-end" />
            </Button>
            <Badge variant="secondary">+938 -664</Badge>
            <Button size="icon-sm" variant="ghost" aria-label="More actions">
              <MoreHorizontalIcon />
            </Button>
          </div>
        </header>

        <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(430px,42vw)]">
          <section className="flex min-w-0 flex-col">
            <div className="flex-1 overflow-y-auto px-6 py-5">
              <div className="mx-auto flex max-w-3xl flex-col gap-5">
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
                      I found a likely authorization bypass in the billing route
                      group. Codex, Claude Code, Gemini, and Local Verifier
                      agree on the affected line range and there is supporting
                      evidence from route setup, middleware, and tests.
                    </p>
                  </div>
                </div>

                <div className="bg-surface-raised rounded-lg border">
                  <div className="flex items-center justify-between border-b px-3 py-2 text-sm">
                    <span className="font-medium">4 files changed</span>
                    <button
                      className="text-muted-foreground hover:text-foreground"
                      type="button"
                    >
                      Undo
                    </button>
                  </div>
                  {changedFiles.map((file) => (
                    <div
                      key={file.path}
                      className="flex items-center justify-between border-b px-3 py-2 text-sm last:border-b-0"
                    >
                      <span className="truncate font-mono text-xs">
                        {file.path}
                      </span>
                      <span className="flex items-center gap-2 text-xs">
                        <span className="text-success">+{file.additions}</span>
                        <span className="text-destructive">
                          -{file.deletions}
                        </span>
                      </span>
                    </div>
                  ))}
                </div>

                <div className="bg-surface-raised rounded-lg border">
                  <div className="flex items-center justify-between border-b px-3 py-2">
                    <div className="flex items-center gap-2">
                      <ShieldCheckIcon />
                      <span className="text-sm font-medium">
                        Evidence-backed findings
                      </span>
                    </div>
                    <Badge variant="secondary">18 total</Badge>
                  </div>
                  {findings.map((finding) => (
                    <FindingRow key={finding.title} finding={finding} />
                  ))}
                </div>
              </div>
            </div>

            <div className="bg-surface-raised border-t p-4">
              <div className="bg-background mx-auto max-w-3xl rounded-2xl border shadow-sm">
                <div className="text-muted-foreground min-h-20 px-4 py-3 text-sm">
                  Ask a follow-up grounded in this evidence bundle...
                </div>
                <div className="flex items-center justify-between border-t px-3 py-2">
                  <div className="flex items-center gap-2">
                    <Button size="sm" variant="ghost">
                      <MessageSquareIcon data-icon="inline-start" />
                      Review
                    </Button>
                    <Button size="sm" variant="ghost">
                      GPT-5.5 Fast
                    </Button>
                    <Button size="sm" variant="ghost">
                      Low
                    </Button>
                  </div>
                  <Button size="icon-sm" aria-label="Send follow-up">
                    <ArrowUpIcon />
                  </Button>
                </div>
              </div>
              <div className="text-muted-foreground mx-auto mt-2 max-w-3xl truncate text-center text-xs">
                {backendUrl || "Waiting for backend info"}
              </div>
            </div>
          </section>

          <aside className="bg-surface-raised min-w-0 border-l">
            <div className="flex h-full flex-col">
              <div className="flex h-12 items-center justify-between border-b px-4">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <Code2Icon />
                  Review
                </div>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  aria-label="Pause review"
                >
                  <PauseIcon />
                </Button>
              </div>

              <div className="flex items-center gap-2 border-b px-4 py-2 text-xs">
                <Badge variant="outline">Unstaged</Badge>
                <Badge variant="secondary">4</Badge>
                <span className="text-muted-foreground">billing.go</span>
                <span className="text-success">+230</span>
                <span className="text-destructive">-150</span>
              </div>

              <div className="min-h-0 flex-1 overflow-y-auto font-mono text-xs">
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
              </div>

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
                    Billing endpoints are reachable without RequireAuth on the
                    route group.
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
        </div>
      </section>
    </main>
  );
}

function SidebarSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="mt-5 flex flex-col gap-1 px-2">
      <div className="text-sidebar-muted flex h-7 items-center justify-between px-2 text-xs">
        <span>{title}</span>
        <SquareIcon />
      </div>
      {children}
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
