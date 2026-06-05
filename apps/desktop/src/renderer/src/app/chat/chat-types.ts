export type ChatAudience = "orchestrator" | "all_agents" | "selected_agent";

export type ChatResponderOption = {
  id: string;
  label: string;
  description: string;
  agentConfigId?: string;
  icon: "orchestrator" | "agent";
  logoUrl?: string;
};

export type ChatAskTargetOption = {
  id: ChatAudience;
  label: string;
  description: string;
  icon: "orchestrator" | "all" | "agent";
};
