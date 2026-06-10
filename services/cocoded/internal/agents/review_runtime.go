package agents

import (
	"fmt"
	"strings"
)

func EnforceReviewModeRuntime(config ConnectionConfig, capabilities AgentCapabilities) (ConnectionConfig, error) {
	if capabilities.empty() {
		capabilities = DefaultCapabilities(config.Kind)
	}
	if capabilities.CanWrite {
		return ConnectionConfig{}, fmt.Errorf("review mode denies write-capable agent runtime")
	}
	if config.CommandSafety.AllowRiskyCommand {
		return ConnectionConfig{}, fmt.Errorf("review mode denies allow_risky_command")
	}

	commandName := commandBaseName(config.Command)
	args := append([]string(nil), config.Args...)
	if commandName == "codex" {
		config.Args = enforceCodexReadOnlyArgs(args)
		config.CommandSafety.AllowRiskyCommand = false
		return config, nil
	}
	for _, arg := range args {
		normalized := strings.ToLower(strings.TrimSpace(arg))
		if normalized == "" {
			continue
		}
		if isReviewModeUnsafeArg(normalized) {
			return ConnectionConfig{}, fmt.Errorf("review mode denies unsafe runtime argument %q", arg)
		}
	}
	config.Args = args
	config.CommandSafety.AllowRiskyCommand = false
	return config, nil
}

func enforceCodexReadOnlyArgs(args []string) []string {
	out := make([]string, 0, len(args)+2)
	hasSandbox := false
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "" {
			continue
		}
		normalized := strings.ToLower(arg)
		switch normalized {
		case "--add-dir":
			if index+1 < len(args) {
				index++
			}
			continue
		case "--sandbox":
			hasSandbox = true
			out = append(out, "--sandbox", "read-only")
			if index+1 < len(args) && !strings.HasPrefix(strings.TrimSpace(args[index+1]), "-") {
				index++
			}
			continue
		default:
			if isReviewModeUnsafeArg(normalized) {
				continue
			}
			out = append(out, arg)
		}
	}
	if !hasSandbox {
		out = insertBeforeFinalPromptArg(out, []string{"--sandbox", "read-only"})
	}
	return out
}

func insertBeforeFinalPromptArg(args []string, inserted []string) []string {
	if len(inserted) == 0 {
		return append([]string(nil), args...)
	}
	if len(args) == 0 || args[len(args)-1] != "-" {
		return append(append([]string(nil), args...), inserted...)
	}
	out := make([]string, 0, len(args)+len(inserted))
	out = append(out, args[:len(args)-1]...)
	out = append(out, inserted...)
	out = append(out, args[len(args)-1])
	return out
}

func isReviewModeUnsafeArg(arg string) bool {
	switch arg {
	case "workspace-write", "danger-full-access", "--dangerously-skip-permissions", "--skip-permissions":
		return true
	default:
		return strings.Contains(arg, "dangerously-skip-permissions")
	}
}

func reviewModeArgsContain(args []string, values ...string) bool {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		for _, value := range values {
			if trimmed == value {
				return true
			}
		}
	}
	return false
}
