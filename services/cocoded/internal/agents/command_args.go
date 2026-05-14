package agents

import (
	"encoding/json"
	"fmt"
	"strings"
)

func DecodeStringArray(raw string, field string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "[]"
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("%s must be a JSON string array", field)
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%s cannot contain empty values", field)
		}
		cleaned = append(cleaned, value)
	}
	return cleaned, nil
}

func CommandArgsWithModelSelection(kind AdapterKind, command string, args []string, modelLabel string, reasoningLabel string) []string {
	out := append([]string(nil), args...)
	if kind != AdapterCLINonInteractive {
		return out
	}
	commandName := commandBaseName(command)
	modelLabel = strings.TrimSpace(modelLabel)
	reasoningLabel = strings.TrimSpace(reasoningLabel)
	if shouldSkipCLIModelArgument(commandName, modelLabel) && reasoningLabel == "" {
		return out
	}
	switch commandName {
	case "codex":
		injected := make([]string, 0, 4)
		if !shouldSkipCLIModelArgument(commandName, modelLabel) {
			injected = append(injected, "--model", modelLabel)
		}
		if reasoningLabel != "" {
			injected = append(injected, "-c", fmt.Sprintf("model_reasoning_effort=%q", reasoningLabel))
			injected = append(injected, "-c", `model_reasoning_summary="auto"`)
			injected = append(injected, "-c", `hide_agent_reasoning=false`)
		}
		return injectArgsAfterSubcommand(out, "exec", injected)
	case "opencode":
		injected := make([]string, 0, 4)
		if !shouldSkipCLIModelArgument(commandName, modelLabel) {
			injected = append(injected, "--model", modelLabel)
		}
		if reasoningLabel != "" {
			injected = append(injected, "--variant", reasoningLabel)
		}
		return injectArgsAfterSubcommand(out, "run", injected)
	case "kiro", "kiro-cli":
		if shouldSkipCLIModelArgument(commandName, modelLabel) {
			return out
		}
		return injectArgsAfterSubcommand(out, "chat", []string{"--model", modelLabel})
	case "claude":
		injected := make([]string, 0, 4)
		if !shouldSkipCLIModelArgument(commandName, modelLabel) {
			injected = append(injected, "--model", modelLabel)
		}
		if reasoningLabel != "" {
			injected = append(injected, "--effort", reasoningLabel)
		}
		return append(injected, out...)
	case "gemini":
		if shouldSkipCLIModelArgument(commandName, modelLabel) {
			return out
		}
		return append([]string{"--model", modelLabel}, out...)
	default:
		return out
	}
}

func SanitizeCommandArgs(command string, args []string) []string {
	commandName := commandBaseName(command)
	out := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "" {
			continue
		}
		if commandName == "claude" && arg == "--tools" && danglingVariadicFlag(args, index) {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func commandBaseName(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	if index := strings.LastIndexAny(command, `/\`); index >= 0 && index+1 < len(command) {
		command = command[index+1:]
	}
	return strings.ToLower(command)
}

func shouldSkipCLIModelArgument(command string, modelLabel string) bool {
	modelLabel = strings.TrimSpace(modelLabel)
	if modelLabel == "" || strings.EqualFold(modelLabel, "default") {
		return true
	}
	switch strings.ToLower(modelLabel) {
	case "codex", "claude", "gemini", "kiro", "kiro-cli", "opencode", "gemini-acp", "opencode-acp":
		return true
	}
	return strings.EqualFold(command, modelLabel)
}

func injectArgsAfterSubcommand(args []string, subcommand string, injected []string) []string {
	if len(injected) == 0 {
		return append([]string(nil), args...)
	}
	out := make([]string, 0, len(args)+len(injected))
	inserted := false
	for _, arg := range args {
		out = append(out, arg)
		if !inserted && arg == subcommand {
			out = append(out, injected...)
			inserted = true
		}
	}
	if inserted {
		return out
	}
	return append(append([]string{}, injected...), args...)
}

func danglingVariadicFlag(args []string, index int) bool {
	if index+1 >= len(args) {
		return true
	}
	next := strings.TrimSpace(args[index+1])
	return next == "" || strings.HasPrefix(next, "-")
}
