import { useEffect, useState } from "react";
import {
  BadgeCheckIcon,
  BotIcon,
  ClipboardListIcon,
  FileCode2Icon,
  GitPullRequestIcon,
  ShieldCheckIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

const threads = [
  { title: "PR #482 billing auth guard", status: "Review running" },
  { title: "PR #479 orderbook refactor", status: "Completed" },
  { title: "PR #476 risk engine limits", status: "Completed" },
];

const steps = [
  "New thread",
  "Configure review",
  "Chat",
  "Findings",
  "Evidence map",
  "Publish",
];

const stats = [
  { label: "Total findings", value: "18", icon: ClipboardListIcon },
  { label: "Verified", value: "6", icon: ShieldCheckIcon },
  { label: "Accepted", value: "3", icon: BadgeCheckIcon },
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
      <aside className="bg-muted/30 flex w-72 shrink-0 flex-col border-r p-5">
        <div className="flex items-center gap-3">
          <div className="bg-primary text-primary-foreground flex size-9 items-center justify-center rounded-lg">
            <BotIcon />
          </div>
          <div>
            <p className="text-lg font-semibold">cocode</p>
            <p className="text-muted-foreground text-sm">
              Review cockpit · {backendStatus}
            </p>
          </div>
        </div>

        <section className="mt-8 flex flex-col gap-3">
          <p className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            Threads
          </p>
          {threads.map((thread) => (
            <button
              key={thread.title}
              className="bg-card hover:bg-accent rounded-lg border px-3 py-3 text-left text-sm shadow-sm transition-colors"
              type="button"
            >
              <span className="block font-medium">{thread.title}</span>
              <span className="text-muted-foreground">{thread.status}</span>
            </button>
          ))}
        </section>
      </aside>

      <section className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-16 items-center justify-between border-b px-6">
          <div className="text-muted-foreground flex items-center gap-3 text-sm">
            <GitPullRequestIcon />
            <span>pharmakon/polymarket-trading</span>
            <Badge variant="secondary">PR #482</Badge>
          </div>
          <div className="flex items-center gap-3">
            <Button variant="outline">Ask all agents</Button>
            <Button>New thread</Button>
          </div>
        </header>

        <div className="flex flex-1 flex-col gap-6 p-6">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-3xl font-semibold">Configure review</h1>
              <Badge>Scaffold</Badge>
            </div>
            <p className="text-muted-foreground mt-2 max-w-2xl">
              The T010-T016 foundation is in place for the cocode desktop shell,
              Go backend, workspace packages, and shadcn-based renderer.
            </p>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Local backend</CardTitle>
              <CardDescription>
                Electron launched cocoded and exposed the connection through the
                preload bridge.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-muted-foreground text-sm">
              {backendUrl || "Waiting for backend info"}
            </CardContent>
          </Card>

          <div className="grid grid-cols-6 gap-3 text-sm">
            {steps.map((step, index) => (
              <div
                key={step}
                className="bg-card rounded-lg border px-3 py-2 text-center"
              >
                <span className="text-muted-foreground">{index + 1}</span>{" "}
                {step}
              </div>
            ))}
          </div>

          <div className="grid grid-cols-3 gap-4">
            {stats.map((stat) => (
              <Card key={stat.label}>
                <CardHeader className="flex flex-row items-center justify-between gap-4">
                  <div>
                    <CardDescription>{stat.label}</CardDescription>
                    <CardTitle className="text-3xl">{stat.value}</CardTitle>
                  </div>
                  <div className="bg-primary/10 text-primary flex size-10 items-center justify-center rounded-lg">
                    <stat.icon />
                  </div>
                </CardHeader>
              </Card>
            ))}
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Foundation ready</CardTitle>
              <CardDescription>
                Next tasks can build the backend API, session state, and
                mock-backed screens against this shell.
              </CardDescription>
            </CardHeader>
            <CardContent className="grid grid-cols-3 gap-4">
              <div className="rounded-lg border p-4">
                <FileCode2Icon className="text-primary mb-3" />
                <p className="font-medium">Electron + React</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  Main, preload, and renderer entrypoints are wired through
                  electron-vite.
                </p>
              </div>
              <div className="rounded-lg border p-4">
                <ShieldCheckIcon className="text-primary mb-3" />
                <p className="font-medium">Go backend</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  The cocoded module exposes a health endpoint with a test.
                </p>
              </div>
              <div className="rounded-lg border p-4">
                <BotIcon className="text-primary mb-3" />
                <p className="font-medium">Agent-ready layout</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  Workspace packages and test fixture directories are ready for
                  schemas, prompts, and fake agents.
                </p>
              </div>
            </CardContent>
          </Card>
        </div>
      </section>
    </main>
  );
}
