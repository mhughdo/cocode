import {
  ActivityIcon,
  BookOpenIcon,
  CheckIcon,
  Code2Icon,
  CopyIcon,
  DatabaseIcon,
  FileSearchIcon,
  FileTextIcon,
  GaugeIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  KeyRoundIcon,
  SearchIcon,
  ShieldCheckIcon,
  type LucideIcon,
} from "lucide-react";

import type {
  AgentConfig,
  Repository,
  RepositoryBranch,
  Snapshot,
} from "@/lib/api";
import githubLogoUrl from "../../../../../../../assets/agents/github.svg";

export type SnapshotSource = "github" | "local-changes" | "branch-compare";

export type SetupReviewAgentAssignment = {
  id: string;
  agent: AgentConfig;
  role: SetupReviewRoleOption;
  index: number;
  manual?: boolean;
};

export type ManualReviewAgentAssignment = {
  id: string;
  agentId: string;
  roleId: string;
};

export const setupSourceOptions: Array<{
  id: SnapshotSource;
  label: string;
  icon: LucideIcon;
  logoUrl?: string;
}> = [
  {
    id: "github",
    label: "GitHub PR URL",
    icon: GitPullRequestIcon,
    logoUrl: githubLogoUrl,
  },
  { id: "local-changes", label: "Local changes", icon: FileTextIcon },
  { id: "branch-compare", label: "Compare branches", icon: GitBranchIcon },
];

export const setupFocusOptions: Array<{
  id: string;
  label: string;
  icon: LucideIcon;
}> = [
  { id: "security", label: "Security issues", icon: ShieldCheckIcon },
  { id: "data", label: "Data leaks", icon: DatabaseIcon },
  { id: "quality", label: "General quality", icon: Code2Icon },
  { id: "edge", label: "Edge cases", icon: GaugeIcon },
];

export const setupFocusHintById: Record<string, string> = {
  security:
    "Prioritize security issues, unsafe authorization boundaries, secret exposure, and injection risks.",
  data: "Trace sensitive data paths across persistence, logs, telemetry, exports, and error responses.",
  quality:
    "Check correctness, maintainability, error handling, API compatibility, and user-visible regressions.",
  edge: "Exercise edge cases around empty input, large diffs, retries, cancellation, concurrency, and rollback paths.",
};

export type SetupFocusArea = {
  id: string;
  label: string;
  instruction: string;
};

export type SetupFocusFileMention = {
  path: string;
  name?: string;
  directory?: string;
};

export type SetupPresetOption = {
  id: string;
  subtitle: string;
  title: string;
  icon: LucideIcon;
  tone: string;
  roleIds: string[];
};

export type SetupReviewRoleOption = {
  id: string;
  label: string;
  shortLabel: string;
  description: string;
  icon: LucideIcon;
};

export const setupReviewRoleOptions: SetupReviewRoleOption[] = [
  {
    id: "general-reviewer",
    label: "General Reviewer",
    shortLabel: "General",
    description:
      "Balanced review across correctness, maintainability, risk, and regressions.",
    icon: FileSearchIcon,
  },
  {
    id: "go-correctness",
    label: "Go Correctness & Idioms Reviewer",
    shortLabel: "Go review",
    description:
      "Checks Go control flow, errors, APIs, and idiomatic implementation details.",
    icon: Code2Icon,
  },
  {
    id: "go-performance",
    label: "Go Performance Reviewer",
    shortLabel: "Performance",
    description:
      "Looks for CPU, memory, allocation, I/O, and large-diff hot-path risks.",
    icon: GaugeIcon,
  },
  {
    id: "go-concurrency",
    label: "Go Concurrency Reviewer",
    shortLabel: "Concurrency",
    description:
      "Reviews goroutine, channel, locking, race, cancellation, and timeout behavior.",
    icon: ActivityIcon,
  },
  {
    id: "postgres-query-performance",
    label: "PostgreSQL Query Performance Reviewer",
    shortLabel: "SQL review",
    description:
      "Reviews query plans, indexes, N+1 patterns, scans, and pagination safety.",
    icon: DatabaseIcon,
  },
  {
    id: "postgres-migration-safety",
    label: "PostgreSQL Migration Safety Reviewer",
    shortLabel: "Migration",
    description:
      "Checks migration locks, backfills, data compatibility, and rollback paths.",
    icon: DatabaseIcon,
  },
  {
    id: "security",
    label: "Security Reviewer",
    shortLabel: "Security",
    description:
      "Looks for injection, unsafe defaults, secret exposure, and supply-chain risk.",
    icon: ShieldCheckIcon,
  },
  {
    id: "authz-tenant-isolation",
    label: "AuthZ & Tenant Isolation Reviewer",
    shortLabel: "AuthZ",
    description:
      "Checks permission boundaries, tenant isolation, identity flow, and confused deputy risks.",
    icon: KeyRoundIcon,
  },
  {
    id: "testing-regression",
    label: "Testing & Regression Reviewer",
    shortLabel: "Testing",
    description:
      "Finds missing coverage, regression boundaries, and brittle test assumptions.",
    icon: FileTextIcon,
  },
  {
    id: "evidence-verifier",
    label: "Evidence Verifier",
    shortLabel: "Evidence",
    description:
      "Verifies findings against exact code evidence and removes weak claims.",
    icon: CheckIcon,
  },
  {
    id: "counter-evidence-skeptic",
    label: "Contradiction Verifier",
    shortLabel: "Verifier",
    description:
      "Separates true contradictions from related safeguards, tests, and false-positive leads.",
    icon: SearchIcon,
  },
  {
    id: "finding-synthesizer",
    label: "Finding Synthesizer",
    shortLabel: "Synthesis",
    description: "Merges overlapping findings into a clear review narrative.",
    icon: BookOpenIcon,
  },
  {
    id: "copy-fix-packet-writer",
    label: "Copy Fix Packet Writer",
    shortLabel: "Fix packet",
    description:
      "Turns verified findings into concise, actionable repair packets.",
    icon: CopyIcon,
  },
];

export const setupPresetOptions: SetupPresetOption[] = [
  {
    id: "standard-pr-review",
    title: "Standard Review",
    subtitle: "Default protocol",
    icon: FileSearchIcon,
    tone: "border-sky-200 bg-sky-50 text-sky-700",
    roleIds: [
      "general-reviewer",
      "go-correctness",
      "security",
      "testing-regression",
      "evidence-verifier",
    ],
  },
  {
    id: "security-auth-focus",
    title: "Security & Auth",
    subtitle: "Auth, tenant, secrets",
    icon: ShieldCheckIcon,
    tone: "border-emerald-200 bg-emerald-50 text-emerald-700",
    roleIds: [
      "security",
      "authz-tenant-isolation",
      "general-reviewer",
      "evidence-verifier",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "go-performance-deep-dive",
    title: "Performance",
    subtitle: "CPU, memory, I/O",
    icon: GaugeIcon,
    tone: "border-violet-200 bg-violet-50 text-violet-700",
    roleIds: [
      "go-performance",
      "go-concurrency",
      "general-reviewer",
      "evidence-verifier",
    ],
  },
  {
    id: "postgres-query-performance",
    title: "SQL Review",
    subtitle: "SQL and indexes",
    icon: DatabaseIcon,
    tone: "border-blue-200 bg-blue-50 text-blue-700",
    roleIds: [
      "postgres-query-performance",
      "general-reviewer",
      "testing-regression",
      "evidence-verifier",
    ],
  },
  {
    id: "postgres-migration-safety",
    title: "Migrations",
    subtitle: "Locks and backfills",
    icon: DatabaseIcon,
    tone: "border-cyan-200 bg-cyan-50 text-cyan-700",
    roleIds: [
      "postgres-migration-safety",
      "postgres-query-performance",
      "general-reviewer",
      "testing-regression",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "data-integrity-transactions",
    title: "Integrity",
    subtitle: "Money, ledgers, writes",
    icon: KeyRoundIcon,
    tone: "border-amber-200 bg-amber-50 text-amber-700",
    roleIds: [
      "general-reviewer",
      "go-correctness",
      "postgres-migration-safety",
      "testing-regression",
      "evidence-verifier",
    ],
  },
  {
    id: "reliability-production-readiness",
    title: "Reliability",
    subtitle: "Timeouts and queues",
    icon: ActivityIcon,
    tone: "border-lime-200 bg-lime-50 text-lime-700",
    roleIds: [
      "go-concurrency",
      "go-performance",
      "general-reviewer",
      "testing-regression",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "testing-regression-coverage",
    title: "Testing",
    subtitle: "Risk and boundaries",
    icon: FileTextIcon,
    tone: "border-stone-200 bg-stone-50 text-stone-700",
    roleIds: [
      "testing-regression",
      "general-reviewer",
      "evidence-verifier",
      "counter-evidence-skeptic",
    ],
  },
  {
    id: "api-compatibility-client-impact",
    title: "API Impact",
    subtitle: "Contracts and SDKs",
    icon: Code2Icon,
    tone: "border-indigo-200 bg-indigo-50 text-indigo-700",
    roleIds: [
      "general-reviewer",
      "go-correctness",
      "testing-regression",
      "finding-synthesizer",
      "copy-fix-packet-writer",
    ],
  },
  {
    id: "privacy-sensitive-data",
    title: "Privacy",
    subtitle: "PII, logs, exports",
    icon: KeyRoundIcon,
    tone: "border-rose-200 bg-rose-50 text-rose-700",
    roleIds: [
      "security",
      "authz-tenant-isolation",
      "general-reviewer",
      "evidence-verifier",
      "copy-fix-packet-writer",
    ],
  },
];

export const setupPrimaryPresetIds = [
  "standard-pr-review",
  "security-auth-focus",
  "go-performance-deep-dive",
  "postgres-query-performance",
];

export function setupReviewAgentAssignments(
  agents: AgentConfig[],
  agentIds: string[],
  roleIds: string[],
): SetupReviewAgentAssignment[] {
  const agentById = new Map(agents.map((agent) => [agent.id, agent]));
  const pool = agentIds
    .map((id) => agentById.get(id))
    .filter((agent): agent is AgentConfig => Boolean(agent));
  if (pool.length === 0) {
    return [];
  }
  const roles = roleIds.length > 0 ? roleIds : [];

  return roles.map((roleId, index) => {
    const role =
      setupReviewRoleById(roleId) ??
      setupReviewRoleById("general-reviewer") ??
      setupReviewRoleOptions[0];
    return {
      id: `preset-review-agent:${role.id}:${index}`,
      agent: pool[index % pool.length],
      role,
      index,
      manual: false,
    };
  });
}

export function setupManualReviewAgentAssignments(
  agents: AgentConfig[],
  assignments: ManualReviewAgentAssignment[],
  startIndex: number,
): SetupReviewAgentAssignment[] {
  const agentById = new Map(agents.map((agent) => [agent.id, agent]));
  const rows: SetupReviewAgentAssignment[] = [];
  for (const [index, assignment] of assignments.entries()) {
    const agent = agentById.get(assignment.agentId);
    if (!agent) {
      continue;
    }
    const role =
      setupReviewRoleById(assignment.roleId) ??
      setupReviewRoleById("general-reviewer") ??
      setupReviewRoleOptions[0];
    rows.push({
      id: assignment.id,
      agent,
      role,
      index: startIndex + index,
      manual: true,
    });
  }
  return rows;
}

export function setupReviewRoleById(roleId?: string) {
  return setupReviewRoleOptions.find((role) => role.id === roleId);
}

export function setupRoleIdsForPresets(selectedPresetIds: Set<string>) {
  const roleIds: string[] = [];
  const seen = new Set<string>();
  const selectedPresets = setupPresetOptions.filter((preset) =>
    selectedPresetIds.has(preset.id),
  );
  for (const preset of selectedPresets) {
    for (const roleId of preset.roleIds) {
      if (!seen.has(roleId)) {
        seen.add(roleId);
        roleIds.push(roleId);
      }
    }
  }
  return roleIds;
}

export function setupFocusPrompt({
  files,
  focusAreas,
  prompt,
}: {
  files: SetupFocusFileMention[];
  focusAreas: SetupFocusArea[];
  prompt: string;
}) {
  const sections: string[] = [];
  if (focusAreas.length > 0) {
    sections.push(
      [
        "Review lenses:",
        ...focusAreas.map(
          (area) => `- ${area.label}: ${area.instruction.trim()}`,
        ),
      ].join("\n"),
    );
  }
  const uniqueFiles = uniqueFocusFiles(files);
  if (uniqueFiles.length > 0) {
    sections.push(
      [
        "Context files to read first:",
        ...uniqueFiles.map((file) => `- ${file.path}`),
      ].join("\n"),
    );
  }
  const trimmed = prompt.trim();
  if (trimmed !== "") {
    sections.push(["Additional reviewer context:", trimmed].join("\n"));
  }
  return sections.join("\n\n");
}

export function uniqueFocusFiles(files: SetupFocusFileMention[]) {
  const seen = new Set<string>();
  const unique: SetupFocusFileMention[] = [];
  for (const file of files) {
    const path = file.path.trim();
    if (path === "" || seen.has(path)) {
      continue;
    }
    seen.add(path);
    unique.push({ ...file, path });
  }
  return unique;
}

export function setupDefaultBaseRef(
  repository?: Repository,
  branches: RepositoryBranch[] = [],
) {
  const primary = setupPreferredBranch(branches, ["main", "master"]);
  if (primary) {
    return primary;
  }
  const fallback = setupPreferredBranch(branches, ["develop", "dev", "trunk"]);
  if (fallback) {
    return fallback;
  }
  const repositoryDefault = repository?.default_branch?.trim();
  if (
    repositoryDefault &&
    repositoryDefault !== "HEAD" &&
    branches.some((item) => item.name === repositoryDefault)
  ) {
    return repositoryDefault;
  }
  return repositoryDefault && repositoryDefault !== "HEAD"
    ? repositoryDefault
    : "main";
}

export function setupPreferredBranch(
  branches: RepositoryBranch[],
  names: string[],
) {
  for (const name of names) {
    const local = branches.find(
      (branch) => !branch.remote && branch.name === name,
    );
    if (local) {
      return local.name;
    }
  }
  for (const name of names) {
    const remote = branches.find(
      (branch) =>
        branch.remote &&
        (branch.name === name || branch.name.endsWith(`/${name}`)),
    );
    if (remote) {
      return remote.name;
    }
  }
  return "";
}

export function setupRuntimeLimitSeconds(depth: "quick" | "standard" | "deep") {
  if (depth === "deep") {
    return 2700;
  }
  if (depth === "quick") {
    return 900;
  }
  return 1800;
}

export function setupSourceKey({
  baseRef,
  githubAuthMethod,
  githubUrl,
  headRef,
  repositoryId,
  source,
}: {
  baseRef: string;
  githubAuthMethod: string;
  githubUrl: string;
  headRef: string;
  repositoryId: string;
  source: SnapshotSource;
}) {
  const parts = [repositoryId.trim(), source];
  if (source === "github") {
    parts.push(githubUrl.trim(), githubAuthMethod.trim());
  }
  if (source === "branch-compare") {
    parts.push(baseRef.trim(), headRef.trim());
  }
  return parts.join("\u001f");
}

export function toggleSetValue(values: Set<string>, value: string) {
  const next = new Set(values);
  if (next.has(value)) {
    next.delete(value);
  } else {
    next.add(value);
  }
  return next;
}

export function removeRecordKey<T>(record: Record<string, T>, key: string) {
  if (!(key in record)) {
    return record;
  }
  const next = { ...record };
  delete next[key];
  return next;
}

export function snapshotTitle(
  snapshot: Snapshot,
  repository?: Repository,
): string {
  if (snapshot.pr_title) {
    return snapshot.pr_title;
  }
  if (snapshot.pr_number && snapshot.owner && snapshot.repo) {
    return `${snapshot.owner}/${snapshot.repo}#${snapshot.pr_number}`;
  }
  if (snapshot.source_type === "branch_compare") {
    return `${repository?.name ?? "Repository"} ${snapshot.base_ref ?? "base"}..${snapshot.head_ref ?? "head"}`;
  }
  if (snapshot.source_type === "local_changes") {
    return `${repository?.name ?? "Repository"} local changes`;
  }
  return `Review ${snapshot.id}`;
}
