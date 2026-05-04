package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCommandHealthReportsAvailableWithVersion(t *testing.T) {
	t.Parallel()

	command := writeFakeHealthCommand(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'fake-agent 1.2.3\n'
  exit 0
fi
exit 0
`)
	health := CheckCommandHealth(context.Background(), healthConfig(command), CommandHealthSettings{VersionTimeoutSeconds: 15})
	if health.Status != HealthAvailable ||
		!strings.Contains(health.Message, "fake-agent 1.2.3") ||
		health.Metadata["version"] != "fake-agent 1.2.3" ||
		health.Metadata["resolved_path"] == "" {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthReportsMissingCommand(t *testing.T) {
	t.Parallel()

	health := CheckCommandHealth(context.Background(), healthConfig(filepath.Join(t.TempDir(), "missing-agent")), CommandHealthSettings{})
	if health.Status != HealthUnavailable || !strings.Contains(health.Message, "not installed") {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthRejectsRiskyCommandByDefault(t *testing.T) {
	t.Parallel()

	health := CheckCommandHealth(context.Background(), healthConfig("sh"), CommandHealthSettings{})
	if health.Status != HealthUnavailable ||
		health.Message != "agent command is blocked by safety policy" ||
		!strings.Contains(health.Metadata["error"].(string), "blocked by default") {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthAllowsExplicitRiskyCommand(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'safe shell wrapper 1.0\\n'\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	health := CheckCommandHealth(context.Background(), healthConfig(path), CommandHealthSettings{
		AllowRiskyCommand:     true,
		VersionTimeoutSeconds: 15,
	})
	if health.Status != HealthAvailable ||
		health.Metadata["version"] != "safe shell wrapper 1.0" {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthUsesExplicitEnvOnly(t *testing.T) {
	t.Setenv("COCODE_HEALTH_TOKEN", "visible")
	t.Setenv("COCODE_PARENT_SECRET", "hidden")
	command := writeFakeHealthCommand(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'token=%s secret=%s\n' "${COCODE_HEALTH_TOKEN-unset}" "${COCODE_PARENT_SECRET-unset}"
  exit 0
fi
exit 1
`)
	config := healthConfig(command)
	config.Env = map[string]string{"COCODE_HEALTH_TOKEN": "visible"}

	health := CheckCommandHealth(context.Background(), config, CommandHealthSettings{VersionTimeoutSeconds: 15})
	if health.Status != HealthAvailable ||
		health.Metadata["version"] != "token=visible secret=unset" {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthReportsVersionFailureAsDegraded(t *testing.T) {
	t.Parallel()

	command := writeFakeHealthCommand(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'version unsupported\n' >&2
  exit 7
fi
exit 0
`)
	health := CheckCommandHealth(context.Background(), healthConfig(command), CommandHealthSettings{})
	if health.Status != HealthDegraded ||
		!strings.Contains(health.Message, "version check failed") ||
		health.Metadata["version_error"] != "version unsupported" {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthRunsSmokePromptWhenEnabled(t *testing.T) {
	t.Parallel()

	command := writeFakeHealthCommand(t, `#!/bin/sh
input=$(/bin/cat)
if [ "$1" = "smoke" ] && [ "$input" = "hello health" ]; then
  printf 'COCODE_HEALTH_OK\n'
  exit 0
fi
printf 'bad smoke: arg=%s input=%s\n' "$1" "$input" >&2
exit 8
`)
	config := healthConfig(command)
	config.Args = []string{"smoke"}
	health := CheckCommandHealth(context.Background(), config, CommandHealthSettings{
		SkipVersion:         true,
		SmokePromptEnabled:  true,
		SmokePrompt:         "hello health",
		SmokePromptExpected: "COCODE_HEALTH_OK",
	})
	if health.Status != HealthAvailable ||
		health.Message != "command smoke check succeeded" ||
		health.Metadata["smoke_prompt"] != true {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthRequiresExpectedSmokePromptOutput(t *testing.T) {
	t.Parallel()

	command := writeFakeHealthCommand(t, `#!/bin/sh
printf 'wrong marker\n'
`)
	health := CheckCommandHealth(context.Background(), healthConfig(command), CommandHealthSettings{
		SkipVersion:         true,
		SmokePromptEnabled:  true,
		SmokePrompt:         "hello health",
		SmokePromptExpected: "COCODE_HEALTH_OK",
	})
	if health.Status != HealthUnavailable ||
		health.Message != "command smoke check did not include expected output" ||
		health.Metadata["smoke_expected"] != "COCODE_HEALTH_OK" {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthRejectsEchoProneSmokePrompt(t *testing.T) {
	t.Parallel()

	command := writeFakeHealthCommand(t, `#!/bin/sh
	/bin/cat
	`)
	health := CheckCommandHealth(context.Background(), healthConfig(command), CommandHealthSettings{
		SkipVersion:         true,
		SmokePromptEnabled:  true,
		SmokePrompt:         "Reply exactly: COCODE_HEALTH_OK",
		SmokePromptExpected: "COCODE_HEALTH_OK",
	})
	if health.Status != HealthUnavailable ||
		health.Message != "smoke prompt includes expected output marker" ||
		health.Metadata["smoke_expected"] != "COCODE_HEALTH_OK" {
		t.Fatalf("health = %+v", health)
	}
}

func TestCheckCommandHealthSmokePromptFailureIsUnavailable(t *testing.T) {
	t.Parallel()

	command := writeFakeHealthCommand(t, "#!/bin/sh\nprintf 'auth failed\\n' >&2\nexit 2\n")
	health := CheckCommandHealth(context.Background(), healthConfig(command), CommandHealthSettings{
		SkipVersion:        true,
		SmokePromptEnabled: true,
		SmokePrompt:        "hello health",
	})
	if health.Status != HealthUnavailable ||
		health.Message != "command smoke check failed" ||
		health.Metadata["error"] != "auth failed" {
		t.Fatalf("health = %+v", health)
	}
}

func healthConfig(command string) ConnectionConfig {
	return ConnectionConfig{
		AdapterID: "agent_config_1",
		Kind:      AdapterCLINonInteractive,
		Command:   command,
	}
}

func writeFakeHealthCommand(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-agent")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
