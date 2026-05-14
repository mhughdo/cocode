package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"slices"
)

func TestValidateCommandSafety(t *testing.T) {
	t.Parallel()

	safeCommand := writeFakeCommand(t, "#!/bin/sh\nexit 0\n")
	tests := []struct {
		name    string
		command string
		options CommandSafetyOptions
		wantErr bool
	}{
		{name: "safe executable path", command: safeCommand},
		{name: "safe command name", command: "codex"},
		{name: "inline args rejected", command: "codex --json", wantErr: true},
		{name: "shell metachar rejected", command: "codex; rm -rf /", wantErr: true},
		{name: "shell interpreter rejected by default", command: "sh", wantErr: true},
		{name: "destructive command rejected by default", command: "rm", wantErr: true},
		{name: "explicit risky shell allowed", command: "sh", options: CommandSafetyOptions{AllowRiskyCommand: true}},
		{name: "explicit risky destructive command allowed", command: "rm", options: CommandSafetyOptions{AllowRiskyCommand: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCommandSafety(tt.command, tt.options)
			if tt.wantErr && err == nil {
				t.Fatal("ValidateCommandSafety() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateCommandSafety() error = %v", err)
			}
		})
	}
}

func TestNormalizeEnvAllowlist(t *testing.T) {
	t.Parallel()

	got, err := NormalizeEnvAllowlist([]string{" OPENAI_API_KEY ", "OPENAI_API_KEY", "_COCODE_TOKEN"})
	if err != nil {
		t.Fatalf("NormalizeEnvAllowlist() error = %v", err)
	}
	if len(got) != 2 || got[0] != "OPENAI_API_KEY" || got[1] != "_COCODE_TOKEN" {
		t.Fatalf("NormalizeEnvAllowlist() = %+v", got)
	}

	for _, values := range [][]string{
		{""},
		{"1BAD"},
		{"BAD-NAME"},
		{"BAD=NAME"},
	} {
		if _, err := NormalizeEnvAllowlist(values); err == nil || !strings.Contains(err.Error(), "environment variable") {
			t.Fatalf("NormalizeEnvAllowlist(%+v) error = %v, want env name error", values, err)
		}
	}
}

func TestDecodeStringArray(t *testing.T) {
	t.Parallel()

	got, err := DecodeStringArray(`[" one ", "two", "two"]`, "agent args")
	if err != nil {
		t.Fatalf("DecodeStringArray() error = %v", err)
	}
	want := []string{"one", "two", "two"}
	if !slices.Equal(got, want) {
		t.Fatalf("DecodeStringArray() = %#v, want %#v", got, want)
	}

	if _, err := DecodeStringArray(`[""]`, "agent args"); err == nil || !strings.Contains(err.Error(), "cannot contain empty values") {
		t.Fatalf("DecodeStringArray(empty) error = %v", err)
	}
}

func TestDecodeStringArrayPreservesDuplicateCommandValues(t *testing.T) {
	t.Parallel()

	got, err := DecodeStringArray(`["-a","never","exec","--color","never","-"]`, "agent args")
	if err != nil {
		t.Fatalf("DecodeStringArray() error = %v", err)
	}
	want := []string{"-a", "never", "exec", "--color", "never", "-"}
	if !slices.Equal(got, want) {
		t.Fatalf("DecodeStringArray() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCLIEnvironmentDefaultsTerminalForKnownCLIs(t *testing.T) {
	t.Parallel()

	got := NormalizeCLIEnvironment("gemini", map[string]string{
		"COLORTERM":   "24bit",
		"FORCE_COLOR": "1",
		"PATH":        "/bin",
		"TERM":        "dumb",
	})
	if got["TERM"] != "xterm-256color" ||
		got["COLORTERM"] != "truecolor" ||
		got["NO_COLOR"] != "1" {
		t.Fatalf("NormalizeCLIEnvironment() = %#v", got)
	}
	if !strings.Contains(got["PATH"], "/usr/bin") || !strings.Contains(got["PATH"], "/bin") {
		t.Fatalf("NormalizeCLIEnvironment(gemini) PATH = %q, want macOS system paths included", got["PATH"])
	}
	if _, ok := got["FORCE_COLOR"]; ok {
		t.Fatalf("NormalizeCLIEnvironment(gemini) = %#v, want FORCE_COLOR removed to avoid Node color warnings", got)
	}
	if _, ok := got["GEMINI_CLI_INTEGRATION_TEST"]; ok {
		t.Fatalf("NormalizeCLIEnvironment(gemini) = %#v, want no integration-test flag", got)
	}
	if !strings.Contains(got["NODE_OPTIONS"], "--no-deprecation") {
		t.Fatalf("NormalizeCLIEnvironment(gemini) NODE_OPTIONS = %q, want --no-deprecation", got["NODE_OPTIONS"])
	}
	if got["GEMINI_PTY_INFO"] != "child_process" {
		t.Fatalf("NormalizeCLIEnvironment(gemini) GEMINI_PTY_INFO = %q, want child_process", got["GEMINI_PTY_INFO"])
	}
	withNodeOptions := NormalizeCLIEnvironment("gemini", map[string]string{"NODE_OPTIONS": "--max-old-space-size=4096"})
	if !strings.Contains(withNodeOptions["NODE_OPTIONS"], "--max-old-space-size=4096") ||
		!strings.Contains(withNodeOptions["NODE_OPTIONS"], "--no-deprecation") {
		t.Fatalf("NormalizeCLIEnvironment(gemini) NODE_OPTIONS = %q, want existing option plus --no-deprecation", withNodeOptions["NODE_OPTIONS"])
	}

	codex := NormalizeCLIEnvironment("codex", map[string]string{"TERM": ""})
	if _, ok := codex["GEMINI_CLI_INTEGRATION_TEST"]; ok {
		t.Fatalf("NormalizeCLIEnvironment(codex) = %#v, want no Gemini integration flag", codex)
	}
	if codex["RUST_LOG"] != "off" {
		t.Fatalf("NormalizeCLIEnvironment(codex) RUST_LOG = %q, want off", codex["RUST_LOG"])
	}
	if codex["USER"] == "" || codex["LOGNAME"] == "" || codex["SHELL"] == "" || codex["TMPDIR"] == "" {
		t.Fatalf("NormalizeCLIEnvironment(codex) identity env = %#v, want USER/LOGNAME/SHELL/TMPDIR", codex)
	}

	kiro := NormalizeCLIEnvironment("kiro-cli", map[string]string{"FORCE_COLOR": "1", "TERM": ""})
	if kiro["TERM"] != "xterm-256color" ||
		kiro["COLORTERM"] != "truecolor" ||
		kiro["NO_COLOR"] != "1" ||
		kiro["USER"] == "" ||
		kiro["LOGNAME"] == "" ||
		kiro["SHELL"] == "" ||
		kiro["TMPDIR"] == "" {
		t.Fatalf("NormalizeCLIEnvironment(kiro-cli) = %#v, want known CLI terminal and identity env", kiro)
	}
	if _, ok := kiro["FORCE_COLOR"]; ok {
		t.Fatalf("NormalizeCLIEnvironment(kiro-cli) = %#v, want FORCE_COLOR removed", kiro)
	}

	unknown := NormalizeCLIEnvironment("custom-reviewer", map[string]string{"TERM": "dumb"})
	if unknown["TERM"] != "dumb" ||
		unknown["COLORTERM"] != "" ||
		unknown["NO_COLOR"] != "" ||
		unknown["FORCE_COLOR"] != "" {
		t.Fatalf("NormalizeCLIEnvironment(unknown) = %#v", unknown)
	}
}

func TestNormalizeCLIEnvironmentPrefersCurrentGoToolBins(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	currentGoBin := filepath.Join(home, "go", "1.26.1", "bin")
	legacyGoBin := filepath.Join(home, "go", "bin")
	goenvShims := filepath.Join(home, ".goenv", "shims")
	for _, dir := range []string{currentGoBin, legacyGoBin, goenvShims} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}

	got := NormalizeCLIEnvironment("codex", map[string]string{
		"HOME": home,
		"PATH": strings.Join([]string{legacyGoBin, "/bin"}, string(os.PathListSeparator)),
	})
	parts := filepath.SplitList(got["PATH"])
	currentIndex := slices.Index(parts, currentGoBin)
	legacyIndex := slices.Index(parts, legacyGoBin)
	goenvIndex := slices.Index(parts, goenvShims)
	if currentIndex < 0 || legacyIndex < 0 || goenvIndex < 0 {
		t.Fatalf("PATH = %q, want current Go bin, legacy Go bin, and goenv shims", got["PATH"])
	}
	if currentIndex > legacyIndex {
		t.Fatalf("PATH = %q, want versioned Go tool bin before legacy GOPATH bin", got["PATH"])
	}
}

func TestPrepareCommandRuntimeEnvironmentAddsWritableToolCaches(t *testing.T) {
	base := t.TempDir()
	t.Setenv("COCODE_AGENT_RUNTIME_DIR", base)
	home := t.TempDir()
	goenvRoot := filepath.Join(home, ".goenv")
	if err := os.MkdirAll(goenvRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll(goenv root) error = %v", err)
	}

	got, cleanup, err := PrepareCommandRuntimeEnvironment("codex", map[string]string{
		"HOME": home,
		"PATH": "/bin",
		"TERM": "",
	})
	if err != nil {
		t.Fatalf("PrepareCommandRuntimeEnvironment(codex) error = %v", err)
	}
	runDir := got["TMPDIR"]
	if runDir == "" || !strings.HasPrefix(runDir, base+string(os.PathSeparator)) {
		t.Fatalf("TMPDIR = %q, want per-run dir under %q", runDir, base)
	}
	for _, key := range []string{"TMP", "TEMP", "XDG_CACHE_HOME", "GOCACHE", "GOPLSCACHE"} {
		if got[key] == "" {
			t.Fatalf("%s was not set in runtime env: %#v", key, got)
		}
		if _, err := os.Stat(got[key]); err != nil {
			t.Fatalf("runtime env %s path %q stat error = %v", key, got[key], err)
		}
	}
	if got["GOTOOLCHAIN"] != "auto" {
		t.Fatalf("GOTOOLCHAIN = %q, want auto", got["GOTOOLCHAIN"])
	}
	if got["GOENV_ROOT"] != goenvRoot {
		t.Fatalf("GOENV_ROOT = %q, want %q", got["GOENV_ROOT"], goenvRoot)
	}
	if runtime.GOOS == "darwin" && (got["DARWIN_USER_TEMP_DIR"] != runDir || got["DARWIN_USER_CACHE_DIR"] == "") {
		t.Fatalf("darwin runtime dirs = %#v", got)
	}
	cleanup()
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("runtime dir stat after cleanup = %v, want missing", err)
	}
}

func TestPrepareCommandRuntimeEnvironmentIsolatesGeminiHome(t *testing.T) {
	t.Parallel()

	sourceHome := t.TempDir()
	sourceGemini := filepath.Join(sourceHome, ".gemini")
	if err := os.MkdirAll(sourceGemini, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceGemini, "GEMINI.md"), []byte("stale global prompt memory"), 0o600); err != nil {
		t.Fatalf("WriteFile(GEMINI.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceGemini, "oauth_creds.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(oauth) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceGemini, "settings.json"), []byte(`{
  "security": { "auth": { "selectedType": "oauth-personal" } },
  "context": { "fileName": "GEMINI.md", "includeDirectoryTree": true },
  "experimental": { "jitContext": true }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	got, cleanup, err := PrepareCommandRuntimeEnvironment("gemini", map[string]string{
		"HOME": sourceHome,
		"PATH": "/bin",
		"TERM": "dumb",
	})
	if err != nil {
		t.Fatalf("PrepareCommandRuntimeEnvironment() error = %v", err)
	}
	isolatedHome := got["HOME"]
	if isolatedHome == "" || isolatedHome == sourceHome {
		t.Fatalf("PrepareCommandRuntimeEnvironment() HOME = %q, want isolated temp home", isolatedHome)
	}
	defer cleanup()

	isolatedGemini := filepath.Join(isolatedHome, ".gemini")
	if _, err := os.Stat(filepath.Join(isolatedGemini, "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatalf("isolated GEMINI.md stat error = %v, want missing", err)
	}
	if data, err := os.ReadFile(filepath.Join(isolatedGemini, geminiIsolatedContextFileName)); err != nil || len(data) != 0 {
		t.Fatalf("isolated context file = %q, err = %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(isolatedGemini, "oauth_creds.json")); err != nil || string(data) != `{"token":"secret"}` {
		t.Fatalf("isolated oauth = %q, err = %v", data, err)
	}
	settingsData, err := os.ReadFile(filepath.Join(isolatedGemini, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}
	contextSettings := settings["context"].(map[string]any)
	if contextSettings["fileName"] != geminiIsolatedContextFileName || contextSettings["includeDirectoryTree"] != false {
		t.Fatalf("isolated context settings = %#v", contextSettings)
	}
	experimental := settings["experimental"].(map[string]any)
	if experimental["jitContext"] != false {
		t.Fatalf("isolated experimental settings = %#v", experimental)
	}
	tools := settings["tools"].(map[string]any)
	if tools["useRipgrep"] != false {
		t.Fatalf("isolated tools settings = %#v", tools)
	}
	shell := tools["shell"].(map[string]any)
	if shell["enableInteractiveShell"] != false || shell["showColor"] != false {
		t.Fatalf("isolated shell settings = %#v", shell)
	}
	security := settings["security"].(map[string]any)
	auth := security["auth"].(map[string]any)
	if auth["selectedType"] != "oauth-personal" {
		t.Fatalf("isolated auth settings = %#v", auth)
	}
	if got["TERM"] != "xterm-256color" {
		t.Fatalf("runtime env = %#v", got)
	}
	if got["GEMINI_PTY_INFO"] != "child_process" {
		t.Fatalf("runtime env GEMINI_PTY_INFO = %q, want child_process", got["GEMINI_PTY_INFO"])
	}

	cleanup()
	if _, err := os.Stat(isolatedHome); !os.IsNotExist(err) {
		t.Fatalf("isolated home stat error = %v, want missing after cleanup", err)
	}
}

func TestPrepareCommandRuntimeEnvironmentLeavesOtherCLIsOnRealHome(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"codex", "kiro-cli"} {
		command := command
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			got, cleanup, err := PrepareCommandRuntimeEnvironment(command, map[string]string{"HOME": home, "TERM": ""})
			if err != nil {
				t.Fatalf("PrepareCommandRuntimeEnvironment(%s) error = %v", command, err)
			}
			defer cleanup()
			if got["HOME"] != home {
				t.Fatalf("PrepareCommandRuntimeEnvironment(%s) HOME = %q, want %q", command, got["HOME"], home)
			}
			if got["TERM"] != "xterm-256color" {
				t.Fatalf("PrepareCommandRuntimeEnvironment(%s) TERM = %q", command, got["TERM"])
			}
			if got["TMPDIR"] == "" || got["XDG_CACHE_HOME"] == "" {
				t.Fatalf("PrepareCommandRuntimeEnvironment(%s) runtime dirs = %#v", command, got)
			}
		})
	}
}

func TestResolveCommandExecutableFindsOpenCodePnpmPlatformBinary(t *testing.T) {
	home := t.TempDir()

	platformPackage := opencodePlatformPackage()
	commandPath := filepath.Join(
		home,
		"Library",
		"pnpm",
		"global",
		"5",
		".pnpm",
		platformPackage+"@1.2.3",
		"node_modules",
		platformPackage,
		"bin",
		"opencode",
	)
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ResolveCommandExecutableWithEnv("opencode", map[string]string{
		"HOME": home,
		"PATH": "",
	})
	if err != nil {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) error = %v", err)
	}
	if got != commandPath {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) = %q, want %q", got, commandPath)
	}
}

func TestResolveCommandExecutablePrefersOpenCodePathExecutableOverPackagedFallback(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	commandPath := filepath.Join(binDir, "opencode")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin dir) error = %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(command) error = %v", err)
	}
	platformPackage := opencodePlatformPackage()
	fallbackPath := filepath.Join(
		home,
		"Library",
		"pnpm",
		"global",
		"5",
		".pnpm",
		platformPackage+"@1.2.3",
		"node_modules",
		platformPackage,
		"bin",
		"opencode",
	)
	if err := os.MkdirAll(filepath.Dir(fallbackPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(platform dir) error = %v", err)
	}
	if err := os.WriteFile(fallbackPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(platform) error = %v", err)
	}

	got, err := ResolveCommandExecutableWithEnv("opencode", map[string]string{
		"HOME": home,
		"PATH": binDir,
	})
	if err != nil {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) error = %v", err)
	}
	if got != commandPath {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) = %q, want PATH executable %q", got, commandPath)
	}
}

func TestResolveCommandExecutableUsesPnpmHomeForOpenCodePlatformBinary(t *testing.T) {
	home := t.TempDir()
	pnpmHome := filepath.Join(home, "custom-pnpm")
	platformPackage := opencodePlatformPackage()
	commandPath := filepath.Join(
		pnpmHome,
		"global",
		"5",
		".pnpm",
		platformPackage+"@1.2.3",
		"node_modules",
		platformPackage,
		"bin",
		"opencode",
	)
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(platform dir) error = %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(platform) error = %v", err)
	}

	got, err := ResolveCommandExecutableWithEnv("opencode", map[string]string{
		"HOME":      home,
		"PATH":      "",
		"PNPM_HOME": pnpmHome,
	})
	if err != nil {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) error = %v", err)
	}
	if got != commandPath {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) = %q, want platform binary %q", got, commandPath)
	}
}

func TestResolveCommandExecutableWithEnvUsesNodeVersionManagerPath(t *testing.T) {
	home := t.TempDir()
	commandPath := filepath.Join(home, ".nvm", "versions", "node", "v24.11.1", "bin", "opencode")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(command dir) error = %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(command) error = %v", err)
	}

	got, err := ResolveCommandExecutableWithEnv("opencode", map[string]string{
		"HOME": home,
		"PATH": "",
	})
	if err != nil {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) error = %v", err)
	}
	if got != commandPath {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) = %q, want %q", got, commandPath)
	}
}

func TestResolveCommandExecutableWithEnvUsesNpmGlobalBin(t *testing.T) {
	home := t.TempDir()
	commandPath := filepath.Join(home, ".npm-global", "bin", "opencode")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(command dir) error = %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(command) error = %v", err)
	}

	got, err := ResolveCommandExecutableWithEnv("opencode", map[string]string{
		"HOME": home,
		"PATH": "",
	})
	if err != nil {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) error = %v", err)
	}
	if got != commandPath {
		t.Fatalf("ResolveCommandExecutableWithEnv(opencode) = %q, want %q", got, commandPath)
	}
}

func TestResolveCommandExecutableWithEnvUsesCommonMacUserPaths(t *testing.T) {
	home := t.TempDir()
	commandPath := filepath.Join(home, ".local", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(command dir) error = %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(command) error = %v", err)
	}

	got, err := ResolveCommandExecutableWithEnv("claude", map[string]string{
		"HOME": home,
		"PATH": "",
	})
	if err != nil {
		t.Fatalf("ResolveCommandExecutableWithEnv(claude) error = %v", err)
	}
	if got != commandPath {
		t.Fatalf("ResolveCommandExecutableWithEnv(claude) = %q, want %q", got, commandPath)
	}
}

func TestCommandArgsWithModelSelection(t *testing.T) {
	t.Parallel()

	got := CommandArgsWithModelSelection(AdapterCLINonInteractive, "claude", []string{"-p", PromptArgPlaceholder}, "claude-sonnet-4-6", "high")
	want := []string{"--model", "claude-sonnet-4-6", "--effort", "high", "-p", PromptArgPlaceholder}
	if !slices.Equal(got, want) {
		t.Fatalf("CommandArgsWithModelSelection() = %#v, want %#v", got, want)
	}
}

func TestCommandArgsWithModelSelectionKeepsGeminiModelFirst(t *testing.T) {
	t.Parallel()

	got := CommandArgsWithModelSelection(
		AdapterCLINonInteractive,
		"gemini",
		[]string{"-p", PromptArgPlaceholder, "--output-format", "json"},
		"gemini-2.5-flash",
		"",
	)
	want := []string{"--model", "gemini-2.5-flash", "-p", PromptArgPlaceholder, "--output-format", "json"}
	if !slices.Equal(got, want) {
		t.Fatalf("CommandArgsWithModelSelection(gemini) = %#v, want %#v", got, want)
	}
}

func TestCommandArgsWithModelSelectionInjectsOpenCodeModelAfterRun(t *testing.T) {
	t.Parallel()

	got := CommandArgsWithModelSelection(
		AdapterCLINonInteractive,
		"opencode",
		[]string{"run", "--pure", "--format", "json", "--thinking", PromptArgPlaceholder},
		"opencode-go/kimi-k2.6",
		"high",
	)
	want := []string{"run", "--model", "opencode-go/kimi-k2.6", "--variant", "high", "--pure", "--format", "json", "--thinking", PromptArgPlaceholder}
	if !slices.Equal(got, want) {
		t.Fatalf("CommandArgsWithModelSelection(opencode) = %#v, want %#v", got, want)
	}
}

func TestCommandArgsWithModelSelectionLeavesKiroHeadlessArgs(t *testing.T) {
	t.Parallel()

	args := []string{"chat", "--no-interactive", "--trust-tools=read,grep,glob,code", PromptArgPlaceholder}
	got := CommandArgsWithModelSelection(AdapterCLINonInteractive, "kiro-cli", args, "kiro", "")
	if !slices.Equal(got, args) {
		t.Fatalf("CommandArgsWithModelSelection(kiro-cli) = %#v, want %#v", got, args)
	}
}

func TestCommandArgsWithModelSelectionInjectsKiroModelAfterChat(t *testing.T) {
	t.Parallel()

	got := CommandArgsWithModelSelection(
		AdapterCLINonInteractive,
		"kiro-cli",
		[]string{"chat", "--no-interactive", "--trust-tools=read,grep,glob,code", PromptArgPlaceholder},
		"claude-sonnet-4.5",
		"",
	)
	want := []string{"chat", "--model", "claude-sonnet-4.5", "--no-interactive", "--trust-tools=read,grep,glob,code", PromptArgPlaceholder}
	if !slices.Equal(got, want) {
		t.Fatalf("CommandArgsWithModelSelection(kiro-cli) = %#v, want %#v", got, want)
	}
}
