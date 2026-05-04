package agents

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

const defaultCommandHealthTimeout = 5 * time.Second
const defaultCommandHealthOutputLimit int64 = 16 << 10

type CommandHealthSettings struct {
	PromptDelivery        PromptDelivery `json:"prompt_delivery,omitempty"`
	AllowRiskyCommand     bool           `json:"allow_risky_command,omitempty"`
	VersionArgs           []string       `json:"version_args,omitempty"`
	SkipVersion           bool           `json:"skip_version,omitempty"`
	SmokePromptEnabled    bool           `json:"smoke_prompt_enabled,omitempty"`
	SmokePrompt           string         `json:"smoke_prompt,omitempty"`
	SmokePromptExpected   string         `json:"smoke_prompt_expected,omitempty"`
	TimeoutSeconds        int64          `json:"timeout_seconds,omitempty"`
	VersionTimeoutSeconds int64          `json:"version_timeout_seconds,omitempty"`
	SmokeTimeoutSeconds   int64          `json:"smoke_timeout_seconds,omitempty"`
}

func CheckCommandHealth(ctx context.Context, config ConnectionConfig, settings CommandHealthSettings) AgentHealth {
	ctx = commandContextOrBackground(ctx)
	checkedAt := time.Now().UTC()
	metadata := map[string]any{
		"adapter_id": config.AdapterID,
		"command":    strings.TrimSpace(config.Command),
	}

	if settings.PromptDelivery != "" {
		config.PromptDelivery = settings.PromptDelivery
	}
	config.PromptDelivery = config.PromptDelivery.Normalize()
	if err := config.Validate(); err != nil {
		return commandHealth(HealthUnavailable, "agent command config is invalid", checkedAt, metadata, err)
	}
	if !supportsCommandHealth(config.Kind) {
		return commandHealth(HealthUnknown, "runtime health checks are not implemented for this adapter kind", checkedAt, metadata, nil)
	}
	if strings.TrimSpace(config.Command) == "" {
		return commandHealth(HealthUnavailable, "command is not configured", checkedAt, metadata, nil)
	}
	if err := ValidateCommandSafety(config.Command, CommandSafetyOptions{AllowRiskyCommand: settings.AllowRiskyCommand}); err != nil {
		return commandHealth(HealthUnavailable, "agent command is blocked by safety policy", checkedAt, metadata, err)
	}
	if err := validateEnv(config.Env); err != nil {
		return commandHealth(HealthUnavailable, "agent command environment is invalid", checkedAt, metadata, err)
	}

	resolvedPath, err := exec.LookPath(config.Command)
	if err != nil {
		return commandHealth(HealthUnavailable, "command is not installed or not on PATH", checkedAt, metadata, err)
	}
	metadata["resolved_path"] = resolvedPath

	status := HealthAvailable
	message := "command is installed"
	if !settings.SkipVersion {
		versionArgs := settings.VersionArgs
		if versionArgs == nil {
			versionArgs = []string{"--version"}
		}
		if len(versionArgs) > 0 {
			stdout, stderr, err := runHealthCommand(ctx, config, versionArgs, "", settings.versionTimeout())
			if err != nil {
				status = HealthDegraded
				message = "command is installed but version check failed"
				metadata["version_error"] = commandErrorMessage(err, stderr)
			} else {
				version := firstNonEmptyLine(stdout)
				if version == "" {
					version = firstNonEmptyLine(stderr)
				}
				if version != "" {
					metadata["version"] = version
					message = "command is installed: " + version
				}
			}
		}
	}

	if settings.SmokePromptEnabled {
		if config.Kind != AdapterCLINonInteractive {
			return commandHealth(HealthDegraded, "protocol adapter smoke prompts are not supported", checkedAt, metadata, nil)
		}
		prompt := settings.SmokePrompt
		if strings.TrimSpace(prompt) == "" {
			prompt = "health check"
		}
		expected := strings.TrimSpace(settings.SmokePromptExpected)
		if expected != "" && strings.Contains(prompt, expected) {
			metadata["smoke_expected"] = expected
			return commandHealth(HealthUnavailable, "smoke prompt includes expected output marker", checkedAt, metadata, nil)
		}
		stdout, stderr, err := runHealthCommand(ctx, config, config.Args, prompt, settings.smokeTimeout())
		if err != nil {
			return commandHealth(HealthUnavailable, "command smoke check failed", checkedAt, metadata, errors.New(commandErrorMessage(err, stderr)))
		}
		if expected != "" &&
			!strings.Contains(stdout, expected) &&
			!strings.Contains(stderr, expected) {
			metadata["smoke_expected"] = expected
			metadata["smoke_stdout"] = truncateMessage(stdout)
			metadata["smoke_stderr"] = truncateMessage(stderr)
			return commandHealth(HealthUnavailable, "command smoke check did not include expected output", checkedAt, metadata, nil)
		}
		status = HealthAvailable
		message = "command smoke check succeeded"
		metadata["smoke_prompt"] = true
	}

	return commandHealth(status, message, checkedAt, metadata, nil)
}

func supportsCommandHealth(kind AdapterKind) bool {
	switch kind {
	case AdapterCLINonInteractive, AdapterJSONRPCStdio, AdapterACPStdio:
		return true
	default:
		return false
	}
}

func commandHealth(status HealthStatus, message string, checkedAt time.Time, metadata map[string]any, err error) AgentHealth {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if err != nil {
		metadata["error"] = err.Error()
	}
	return AgentHealth{
		Status:    status,
		Message:   message,
		CheckedAt: checkedAt,
		Metadata:  metadata,
	}
}

func runHealthCommand(ctx context.Context, config ConnectionConfig, args []string, prompt string, timeout time.Duration) (string, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdinArgs := append([]string(nil), args...)
	delivery := commandPromptDelivery{args: stdinArgs, cleanup: func() {}}
	if prompt != "" {
		promptConfig := config
		promptConfig.Args = stdinArgs
		prepared, err := (&CommandOnceConnection{config: promptConfig}).preparePrompt(AgentTask{Prompt: prompt})
		if err != nil {
			return "", "", err
		}
		delivery = prepared
		defer delivery.cleanup()
	}

	stdout := &limitedOutput{limit: defaultCommandHealthOutputLimit}
	stderr := &limitedOutput{limit: defaultCommandHealthOutputLimit}
	cmd := exec.CommandContext(runCtx, config.Command, delivery.args...)
	if config.WorkingDirectory != "" {
		cmd.Dir = config.WorkingDirectory
	}
	cmd.Env = envList(config.Env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = delivery.stdin

	err := cmd.Run()
	if runCtx.Err() != nil {
		return stdout.String(), stderr.String(), runCtx.Err()
	}
	if err != nil {
		return stdout.String(), stderr.String(), err
	}
	return stdout.String(), stderr.String(), nil
}

func commandErrorMessage(err error, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr != "" {
		return truncateMessage(stderr)
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func (s CommandHealthSettings) versionTimeout() time.Duration {
	if s.VersionTimeoutSeconds > 0 {
		return time.Duration(s.VersionTimeoutSeconds) * time.Second
	}
	if s.TimeoutSeconds > 0 {
		return time.Duration(s.TimeoutSeconds) * time.Second
	}
	return defaultCommandHealthTimeout
}

func (s CommandHealthSettings) smokeTimeout() time.Duration {
	if s.SmokeTimeoutSeconds > 0 {
		return time.Duration(s.SmokeTimeoutSeconds) * time.Second
	}
	if s.TimeoutSeconds > 0 {
		return time.Duration(s.TimeoutSeconds) * time.Second
	}
	return defaultCommandHealthTimeout
}
