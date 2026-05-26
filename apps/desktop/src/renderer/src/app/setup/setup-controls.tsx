import { type ReactNode, useState } from "react";
import {
  CheckIcon,
  ChevronDownIcon,
  CircleIcon,
  GitBranchIcon,
  UsersIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react";

import { LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import type {
  AgentConfig,
  AgentModelCatalog,
  AgentModelOption,
  Loadable,
  RepositoryBranch,
} from "@/lib/api";
import { cn } from "@/lib/utils";

import {
  agentLogoUrl,
  formatSetupAgentCompactChoiceLabel,
  formatSetupAgentModelChoiceLabel,
  groupSetupModelsByProvider,
  setupChoiceForAgent,
  setupChoiceFromModel,
  setupModelsForAgent,
  type AgentVisibilitySource,
  type SetupAgentModelChoice,
} from "../agents/agent-utils";
import type { SetupReviewRoleOption } from "./setup-model";

export function SetupStepPanel({
  children,
  compact = false,
  description,
  number,
  title,
}: {
  children: ReactNode;
  compact?: boolean;
  description: string;
  number: number;
  title: string;
}) {
  return (
    <section className="grid grid-cols-[32px_minmax(0,1fr)] gap-3">
      <div className="relative flex justify-center pt-2.5">
        {number < 4 && (
          <span className="bg-border absolute top-[38px] bottom-[-18px] left-1/2 w-px -translate-x-1/2" />
        )}
        <span className="bg-foreground text-background relative z-10 flex size-7 items-center justify-center rounded-full text-xs font-semibold">
          {number}
        </span>
      </div>
      <div className="bg-card border-border-subtle rounded-xl border px-4 py-3.5">
        <div
          className={cn(
            "grid gap-4",
            compact
              ? "min-[1540px]:grid-cols-[204px_minmax(0,1fr)]"
              : "lg:grid-cols-[204px_minmax(0,1fr)]",
          )}
        >
          <div className="min-w-0">
            <h2 className="text-md leading-5 font-semibold">{title}</h2>
            <p className="text-muted-foreground mt-1 text-sm leading-5">
              {description}
            </p>
          </div>
          <div className="min-w-0">{children}</div>
        </div>
      </div>
    </section>
  );
}

export function SetupSegment({
  active,
  icon: Icon,
  label,
  logoUrl,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  label: string;
  logoUrl?: string;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "bg-surface-sunken text-muted-foreground hover:bg-muted hover:text-foreground flex h-10 min-w-0 cursor-pointer items-center justify-between gap-2 rounded-lg border border-transparent px-2.5 text-left text-sm font-medium transition-colors",
        active && "bg-card text-foreground border-border",
      )}
      type="button"
      onClick={onClick}
    >
      <span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
        {logoUrl ? (
          <img alt="" className="size-4 shrink-0 rounded-[3px]" src={logoUrl} />
        ) : (
          <Icon className="size-3.5 shrink-0" />
        )}
        <span className="min-w-0 truncate">{label}</span>
      </span>
      <span
        className={cn(
          "border-border-strong flex size-3.5 shrink-0 items-center justify-center rounded-full border bg-card",
          active && "border-foreground bg-foreground",
        )}
      >
        {active && <span className="bg-background size-1.5 rounded-full" />}
      </span>
    </button>
  );
}

export function SetupBranchSelector({
  branches,
  disabled,
  label,
  value,
  onSelect,
}: {
  branches: Loadable<RepositoryBranch[]>;
  disabled: boolean;
  label: string;
  value: string;
  onSelect: (value: string) => void;
}) {
  const [query, setQuery] = useState("");
  const options = branches.status === "success" ? branches.data : [];
  const filtered = options.filter((branch) =>
    branch.name.toLowerCase().includes(query.trim().toLowerCase()),
  );
  return (
    <label className="flex min-w-0 flex-col gap-1.5 text-xs font-medium">
      {label}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            aria-label={`${label}: ${value}`}
            className="border-border hover:bg-muted/75 bg-card flex h-9 w-full cursor-pointer items-center justify-between gap-2 rounded-md border px-3 text-left text-sm font-medium shadow-xs disabled:cursor-default disabled:opacity-60"
            disabled={disabled}
            type="button"
          >
            <span className="flex min-w-0 items-center gap-2">
              <GitBranchIcon className="size-3.5 shrink-0" />
              <span className="truncate">{value || "Choose branch"}</span>
            </span>
            <ChevronDownIcon className="text-muted-foreground size-3.5 shrink-0" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-80 p-2">
          <Input
            aria-label={`Search ${label.toLowerCase()}`}
            className="mb-2 h-8"
            placeholder="Search branches..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => event.stopPropagation()}
          />
          <div className="max-h-60 overflow-y-auto pr-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {branches.status === "loading" && (
              <div className="px-2 py-2">
                <LoadingRows rows={3} />
              </div>
            )}
            {branches.status === "error" && (
              <div className="text-destructive px-2 py-2 text-xs leading-5">
                {branches.error.message}
              </div>
            )}
            {branches.status === "success" &&
              filtered.map((branch) => (
                <DropdownMenuItem
                  key={`${branch.remote ? "remote" : "local"}:${branch.name}`}
                  className="cursor-pointer"
                  onSelect={() => onSelect(branch.name)}
                >
                  <GitBranchIcon className="size-3.5" />
                  <span className="min-w-0 flex-1 truncate">{branch.name}</span>
                  {branch.remote && (
                    <Badge
                      className="h-5 px-1.5 text-[0.62rem]"
                      variant="outline"
                    >
                      remote
                    </Badge>
                  )}
                  {branch.name === value && <CheckIcon className="size-3.5" />}
                </DropdownMenuItem>
              ))}
            {branches.status === "success" && filtered.length === 0 && (
              <DropdownMenuItem disabled>No matching branches</DropdownMenuItem>
            )}
          </div>
        </DropdownMenuContent>
      </DropdownMenu>
    </label>
  );
}

export function SetupFocusChip({
  active,
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      aria-pressed={active}
      className={cn(
        "border-transparent text-muted-foreground bg-surface-sunken hover:bg-muted hover:text-foreground inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-full border px-3 text-sm font-medium transition-colors",
        active && "border-border bg-card text-foreground",
      )}
      type="button"
      onClick={onClick}
    >
      <Icon className="size-3.5 shrink-0" />
      <span>{label}</span>
      {active && <CheckIcon className="size-3.5 shrink-0" />}
    </button>
  );
}

export function SetupAgentSelector({
  agents,
  catalogs,
  choices,
  disabled = false,
  placeholder,
  selectedAgentId,
  onSelect,
}: {
  agents: AgentConfig[];
  catalogs: AgentModelCatalog[];
  choices: Record<string, SetupAgentModelChoice>;
  disabled?: boolean;
  placeholder: string;
  selectedAgentId: string;
  onSelect: (agentId: string, choice: SetupAgentModelChoice) => void;
}) {
  const selectedAgent = agents.find((agent) => agent.id === selectedAgentId);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          aria-label={placeholder}
          className="border-border hover:bg-muted/75 bg-card flex h-8 w-full cursor-pointer items-center justify-between gap-3 rounded-md border px-2.5 text-left text-sm font-medium shadow-xs disabled:cursor-default disabled:opacity-60"
          disabled={disabled}
          type="button"
        >
          <span className="flex min-w-0 items-center gap-2">
            {selectedAgent ? (
              <AgentProviderGlyph agent={selectedAgent} />
            ) : (
              <UsersIcon className="size-3.5" />
            )}
            <span className="truncate">
              {selectedAgent
                ? formatSetupAgentCompactChoiceLabel(
                    selectedAgent,
                    choices,
                    catalogs,
                  )
                : placeholder}
            </span>
          </span>
          <ChevronDownIcon className="text-muted-foreground size-3.5 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-80">
        <DropdownMenuLabel>CLI</DropdownMenuLabel>
        <DropdownMenuGroup>
          {agents.map((agent) => (
            <SetupAgentMenuBranch
              key={agent.id}
              agent={agent}
              catalogs={catalogs}
              choices={choices}
              selected={agent.id === selectedAgentId}
              onSelect={onSelect}
            />
          ))}
        </DropdownMenuGroup>
        {agents.length === 0 && (
          <DropdownMenuItem disabled>
            No review-safe agents found
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SetupAgentMenuBranch({
  agent,
  catalogs,
  choices,
  selected,
  onSelect,
}: {
  agent: AgentConfig;
  catalogs: AgentModelCatalog[];
  choices: Record<string, SetupAgentModelChoice>;
  selected: boolean;
  onSelect: (agentId: string, choice: SetupAgentModelChoice) => void;
}) {
  const models = setupModelsForAgent(agent, catalogs);
  if (models.length === 0) {
    return (
      <DropdownMenuItem
        className="cursor-pointer"
        onSelect={() => onSelect(agent.id, {})}
      >
        <AgentProviderGlyph agent={agent} />
        <span className="min-w-0 flex-1 truncate">{agent.name}</span>
        {selected && <CheckIcon className="size-3.5" />}
      </DropdownMenuItem>
    );
  }
  const groups = groupSetupModelsByProvider(models);
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger className="cursor-pointer">
        <AgentProviderGlyph agent={agent} />
        <span className="min-w-0 flex-1 truncate">{agent.name}</span>
        {selected && <CheckIcon className="text-muted-foreground size-3.5" />}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-72">
        {groups.length > 1 ? (
          groups.map((group) => (
            <DropdownMenuSub key={group.provider}>
              <DropdownMenuSubTrigger className="cursor-pointer">
                {group.providerLabel}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="w-72">
                <SetupModelMenuItems
                  agent={agent}
                  choices={choices}
                  models={group.models}
                  onSelect={onSelect}
                />
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          ))
        ) : (
          <SetupModelMenuItems
            agent={agent}
            choices={choices}
            models={groups[0]?.models ?? models}
            onSelect={onSelect}
          />
        )}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

function SetupAgentModelSelector({
  agent,
  catalogs,
  choices,
  className,
  label,
  onSelect,
}: {
  agent: AgentConfig;
  catalogs: AgentModelCatalog[];
  choices: Record<string, SetupAgentModelChoice>;
  className?: string;
  label?: string;
  onSelect: (choice: SetupAgentModelChoice) => void;
}) {
  const models = setupModelsForAgent(agent, catalogs);
  const groups = groupSetupModelsByProvider(models);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          className={cn(
            "bg-surface text-muted-foreground hover:border-border hover:text-foreground flex h-6 min-w-0 cursor-pointer items-center justify-between gap-2 rounded-md border border-transparent px-1.5 text-left text-[0.73rem] font-medium hover:bg-white",
            className,
          )}
          type="button"
        >
          <span className="truncate">
            {label ??
              formatSetupAgentModelChoiceLabel(agent, choices, catalogs)}
          </span>
          <ChevronDownIcon className="size-3 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-72">
        {models.length === 0 ? (
          <DropdownMenuItem disabled>Use configured model</DropdownMenuItem>
        ) : groups.length > 1 ? (
          groups.map((group) => (
            <DropdownMenuSub key={group.provider}>
              <DropdownMenuSubTrigger className="cursor-pointer">
                {group.providerLabel}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="w-72">
                <SetupModelMenuItems
                  agent={agent}
                  choices={choices}
                  models={group.models}
                  onSelect={(_, choice) => onSelect(choice)}
                />
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          ))
        ) : (
          <SetupModelMenuItems
            agent={agent}
            choices={choices}
            models={groups[0]?.models ?? models}
            onSelect={(_, choice) => onSelect(choice)}
          />
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SetupModelMenuItems({
  agent,
  choices,
  models,
  onSelect,
}: {
  agent: AgentConfig;
  choices: Record<string, SetupAgentModelChoice>;
  models: AgentModelOption[];
  onSelect: (agentId: string, choice: SetupAgentModelChoice) => void;
}) {
  const current = setupChoiceForAgent(agent, choices, []);
  return (
    <>
      {models.map((model) => {
        const reasoning = model.reasoning_efforts ?? [];
        if (reasoning.length > 0) {
          return (
            <DropdownMenuSub key={model.id}>
              <DropdownMenuSubTrigger className="cursor-pointer">
                <span className="min-w-0 flex-1 truncate">{model.label}</span>
                {current.modelId === model.id && (
                  <CheckIcon className="text-muted-foreground size-3.5" />
                )}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="w-48">
                <DropdownMenuLabel>Reasoning effort</DropdownMenuLabel>
                {reasoning.map((effort) => (
                  <DropdownMenuItem
                    key={effort.id}
                    className="cursor-pointer"
                    onSelect={() =>
                      onSelect(agent.id, setupChoiceFromModel(model, effort))
                    }
                  >
                    <span className="min-w-0 flex-1 truncate">
                      {effort.label}
                    </span>
                    {current.modelId === model.id &&
                      current.reasoning === effort.id && (
                        <CheckIcon className="size-3.5" />
                      )}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          );
        }
        return (
          <DropdownMenuItem
            key={model.id}
            className="cursor-pointer"
            onSelect={() => onSelect(agent.id, setupChoiceFromModel(model))}
          >
            <span className="min-w-0 flex-1 truncate">{model.label}</span>
            {current.modelId === model.id && <CheckIcon className="size-3.5" />}
          </DropdownMenuItem>
        );
      })}
    </>
  );
}

export function SetupAgentRow({
  agent,
  catalogs,
  checked,
  choices,
  locked = false,
  role,
  roles,
  onCheckedChange,
  onModelChoice,
  onRoleChange,
}: {
  agent: AgentConfig;
  catalogs: AgentModelCatalog[];
  checked: boolean;
  choices: Record<string, SetupAgentModelChoice>;
  locked?: boolean;
  role?: SetupReviewRoleOption;
  roles: SetupReviewRoleOption[];
  onCheckedChange: (checked: boolean) => void;
  onModelChoice: (choice: SetupAgentModelChoice) => void;
  onRoleChange: (roleId: string) => void;
}) {
  return (
    <div
      aria-selected={checked}
      className={cn(
        "border-border-subtle grid min-h-9 grid-cols-[minmax(96px,1fr)_92px_24px] items-center gap-1.5 border-b px-2.5 py-1 text-sm last:border-b-0",
        locked ? "bg-muted/40" : "bg-transparent",
      )}
    >
      <span className="flex min-w-0 items-center gap-2">
        <AgentProviderGlyph agent={agent} />
        <SetupAgentModelSelector
          agent={agent}
          catalogs={catalogs}
          choices={choices}
          className="text-foreground hover:text-foreground h-auto w-full flex-1 border-0 bg-transparent p-0 text-[0.79rem] shadow-none hover:border-transparent hover:bg-transparent"
          label={formatSetupAgentCompactChoiceLabel(agent, choices, catalogs)}
          onSelect={onModelChoice}
        />
      </span>
      {locked ? (
        <span className="text-muted-foreground flex h-6 w-full items-center truncate px-1.5 text-left text-[0.7rem]">
          Orchestrator
        </span>
      ) : (
        <SetupRoleSelector
          role={role ?? roles[0]}
          roles={roles}
          onSelect={onRoleChange}
        />
      )}
      {locked ? (
        <CheckIcon className="text-muted-foreground mx-auto size-3.5" />
      ) : (
        <button
          aria-label={`Remove ${agent.name}`}
          className="text-muted-foreground hover:bg-surface-muted hover:text-foreground flex size-7 cursor-pointer items-center justify-center rounded-md"
          type="button"
          onClick={() => onCheckedChange(false)}
        >
          <XIcon className="size-3.5" />
        </button>
      )}
    </div>
  );
}

function SetupRoleSelector({
  role,
  roles,
  onSelect,
}: {
  role: SetupReviewRoleOption;
  roles: SetupReviewRoleOption[];
  onSelect: (roleId: string) => void;
}) {
  const [query, setQuery] = useState("");
  const normalizedQuery = query.trim().toLowerCase();
  const visibleRoles = normalizedQuery
    ? roles.filter((item) =>
        [
          item.label,
          item.shortLabel,
          item.description,
          item.id.replaceAll("-", " "),
        ]
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery),
      )
    : roles;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          className="text-muted-foreground hover:bg-surface-muted hover:text-foreground grid h-6 w-full cursor-pointer grid-cols-[minmax(0,1fr)_12px] items-center gap-1.5 rounded-md px-1.5 text-left text-[0.7rem]"
          type="button"
        >
          <span className="truncate">{role.shortLabel}</span>
          <ChevronDownIcon className="size-3 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80 p-2">
        <div className="mb-2 px-1">
          <DropdownMenuLabel className="px-0">Reviewer role</DropdownMenuLabel>
          <Input
            aria-label="Search reviewer roles"
            className="mt-1 h-8 text-[0.78rem]"
            placeholder="Search roles..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => event.stopPropagation()}
          />
        </div>
        <div className="max-h-72 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {visibleRoles.map((item) => (
            <DropdownMenuItem
              key={item.id}
              className="cursor-pointer gap-2"
              onSelect={() => onSelect(item.id)}
            >
              <item.icon className="size-3.5 shrink-0" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[0.78rem] font-medium">
                  {item.label}
                </span>
                <span className="text-muted-foreground line-clamp-2 block text-[0.68rem] leading-4">
                  {item.description}
                </span>
              </span>
              {item.id === role.id && <CheckIcon className="size-3.5" />}
            </DropdownMenuItem>
          ))}
          {visibleRoles.length === 0 && (
            <DropdownMenuItem disabled>No matching roles</DropdownMenuItem>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function SetupPresetTile({
  active,
  icon: Icon,
  subtitle,
  tone,
  title,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  subtitle: string;
  tone: string;
  title: string;
  onClick: () => void;
}) {
  return (
    <button
      aria-pressed={active}
      className={cn(
        "bg-card border-border-subtle hover:border-border hover:bg-surface-muted/30 focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none flex min-h-[80px] cursor-pointer items-start gap-3 rounded-lg border px-3.5 py-3 text-left transition-colors",
        active && "border-foreground/30 bg-surface-muted/40",
      )}
      type="button"
      onClick={onClick}
    >
      <span
        className={cn(
          "flex size-7 shrink-0 items-center justify-center rounded-md border",
          tone,
        )}
      >
        <Icon className="size-3.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="line-clamp-2 text-sm leading-snug font-semibold">
          {title}
        </span>
        <span className="text-muted-foreground mt-1 line-clamp-2 text-xs leading-4">
          {subtitle}
        </span>
      </span>
      {active && <CheckIcon className="text-foreground/60 size-3.5 shrink-0" />}
    </button>
  );
}

export function SetupScopeRow({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right font-mono">{value}</span>
    </div>
  );
}

export function AgentProviderGlyph({
  agent,
}: {
  agent: AgentVisibilitySource;
}) {
  const logo = agentLogoUrl(agent);
  if (logo) {
    return (
      <img
        alt=""
        className="size-4 shrink-0 rounded-[3px] object-contain"
        src={logo}
      />
    );
  }
  return <CircleIcon className="size-4 shrink-0" />;
}
