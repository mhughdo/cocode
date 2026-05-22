import { type FormEvent, useState } from "react";
import { ArrowUpIcon, ChevronDownIcon, MessageSquareIcon } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  InputGroup,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import { Button } from "@/components/ui/button";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import type { AgentConfig, Loadable, ReviewContextPolicy } from "@/lib/api";
import { agentEgress, formatSetupAgentLabel } from "../agents/agent-utils";

export type ComposerMode = "review" | "finding follow-up";
export type ComposerRuntime = "quick" | "standard" | "deep";
export type ComposerReasoning = "low" | "medium" | "high";
export type ComposerPermission = "review-mode" | "local-only";

const COMPOSER_RUNTIME_POLICIES = {
  quick: { max_tokens: 4_000, max_items: 40 },
  standard: { max_tokens: 8_000, max_items: 80 },
  deep: { max_tokens: 12_000, max_items: 120 },
} satisfies Record<
  ComposerRuntime,
  Pick<ReviewContextPolicy, "max_tokens" | "max_items">
>;

export function composerContextPolicy(
  runtime: ComposerRuntime,
  permission: ComposerPermission,
): ReviewContextPolicy {
  const limits = COMPOSER_RUNTIME_POLICIES[runtime];
  return {
    ...limits,
    redact_secrets: true,
    include_prior_comments: permission === "review-mode",
    include_prior_decisions: true,
  };
}

export function MessageComposer({
  agents: directAgents,
  agentConfigs,
  backendDetail,
  disabled,
  disabledReason,
  defaultMode = "review",
  onQuestionChange,
  onSelectedAgentIdChange,
  onSubmit,
  question,
  selectedAgentId,
  submitting,
}: {
  agents?: AgentConfig[];
  agentConfigs?: Loadable<AgentConfig[]>;
  backendDetail?: string;
  disabled?: boolean;
  disabledReason?: string;
  defaultMode?: ComposerMode;
  onQuestionChange?: (value: string) => void;
  onSelectedAgentIdChange?: (value: string) => void;
  onSubmit?: (
    question: string,
    options: {
      agentConfigId?: string;
      contextPolicy: ReviewContextPolicy;
      mode: ComposerMode;
      permission: ComposerPermission;
      reasoning: ComposerReasoning;
      runtime: ComposerRuntime;
    },
  ) => void | Promise<void>;
  question?: string;
  selectedAgentId?: string;
  submitting?: boolean;
}) {
  const [mode, setMode] = useState<ComposerMode>(defaultMode);
  const [runtime, setRuntime] = useState<ComposerRuntime>("standard");
  const [reasoning, setReasoning] = useState<ComposerReasoning>("high");
  const [permission, setPermission] =
    useState<ComposerPermission>("review-mode");
  const [draftQuestion, setDraftQuestion] = useState("");
  const allAgents =
    directAgents ??
    (agentConfigs?.status === "success" ? agentConfigs.data : []);
  const safeAgents = allAgents.filter(
    (agent) => agent.enabled && !agent.capabilities.can_write,
  );
  const composerAgents =
    permission === "local-only"
      ? safeAgents.filter((agent) => agentEgress(agent) === "local")
      : safeAgents;
  const selectedAgent = composerAgents.find(
    (agent) => agent.id === selectedAgentId,
  );
  const effectiveQuestion = question ?? draftQuestion;
  const canSubmit =
    Boolean(onSubmit) &&
    !disabled &&
    !submitting &&
    Boolean(effectiveQuestion.trim());
  const detailMessage =
    disabledReason ??
    (onSubmit
      ? backendDetail
      : "Open Follow-up from a finding to send a scoped question.");

  function updateQuestion(value: string) {
    if (onQuestionChange) {
      onQuestionChange(value);
      return;
    }
    setDraftQuestion(value);
  }

  async function submitComposer(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canSubmit || !onSubmit) {
      return;
    }
    const trimmedQuestion = effectiveQuestion.trim();
    await onSubmit(trimmedQuestion, {
      agentConfigId: selectedAgent?.id,
      contextPolicy: composerContextPolicy(runtime, permission),
      mode,
      permission,
      reasoning,
      runtime,
    });
    if (!onQuestionChange) {
      setDraftQuestion("");
    }
  }

  return (
    <div className="bg-surface-raised/95 border-t p-4">
      <form
        className="cocode-panel mx-auto max-w-5xl overflow-hidden"
        onSubmit={(event) => void submitComposer(event)}
      >
        <InputGroup className="min-h-24 items-stretch border-0">
          <InputGroupTextarea
            aria-label="Follow-up prompt"
            disabled={disabled}
            value={effectiveQuestion}
            placeholder={
              disabled
                ? "Start a review before asking follow-up questions..."
                : "Ask a follow-up grounded in this review context..."
            }
            className="min-h-20"
            onChange={(event) => updateQuestion(event.target.value)}
          />
        </InputGroup>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t px-3 py-2">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <ComposerDropdown
              label={`Tool: ${mode}`}
              onSelect={setMode}
              options={["review", "finding follow-up"]}
            />
            {composerAgents.length > 0 && (
              <NativeSelect
                aria-label="Follow-up agent"
                className="max-w-56"
                disabled={disabled || submitting}
                size="sm"
                value={selectedAgent?.id ?? ""}
                onChange={(event) =>
                  onSelectedAgentIdChange?.(event.target.value)
                }
              >
                <NativeSelectOption value="">
                  Auto-select agent
                </NativeSelectOption>
                {composerAgents.map((agent) => (
                  <NativeSelectOption key={agent.id} value={agent.id}>
                    {formatSetupAgentLabel(agent)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            )}
            {composerAgents.length === 0 && (
              <Button disabled size="sm" variant="ghost">
                <MessageSquareIcon data-icon="inline-start" />
                No read-only agents
              </Button>
            )}
            <ComposerDropdown
              label={`Context: ${runtime}`}
              onSelect={setRuntime}
              options={["quick", "standard", "deep"]}
            />
            <ComposerDropdown
              label={`Reasoning: ${reasoning}`}
              onSelect={setReasoning}
              options={["low", "medium", "high"]}
            />
            <ComposerDropdown
              label={`Permission: ${permission}`}
              onSelect={setPermission}
              options={["review-mode", "local-only"]}
            />
          </div>
          <InputGroupButton
            disabled={!canSubmit}
            size="icon-sm"
            type="submit"
            variant={canSubmit ? "default" : "ghost"}
            aria-label="Send follow-up question"
          >
            <ArrowUpIcon />
          </InputGroupButton>
        </div>
      </form>
      {detailMessage && (
        <div className="text-muted-foreground mx-auto mt-2 max-w-5xl truncate text-center text-xs">
          {detailMessage}
        </div>
      )}
    </div>
  );
}

function ComposerDropdown<T extends string>({
  label,
  onSelect,
  options,
}: {
  label: string;
  onSelect?: (value: T) => void;
  options: readonly T[];
}) {
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
          {options.map((option) => (
            <DropdownMenuItem key={option} onSelect={() => onSelect?.(option)}>
              {option}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
