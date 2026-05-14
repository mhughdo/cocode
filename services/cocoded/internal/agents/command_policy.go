package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

type CommandSafetyOptions struct {
	AllowRiskyCommand bool `json:"allow_risky_command,omitempty"`
}

func DecodeCommandSafetyOptionsJSON(raw string) (CommandSafetyOptions, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var options CommandSafetyOptions
	if err := json.Unmarshal([]byte(raw), &options); err != nil {
		return CommandSafetyOptions{}, err
	}
	return options, nil
}

var riskyCommandReasons = map[string]string{
	"sh":             "shell interpreter",
	"bash":           "shell interpreter",
	"zsh":            "shell interpreter",
	"dash":           "shell interpreter",
	"fish":           "shell interpreter",
	"ksh":            "shell interpreter",
	"csh":            "shell interpreter",
	"tcsh":           "shell interpreter",
	"cmd":            "shell interpreter",
	"cmd.exe":        "shell interpreter",
	"powershell":     "shell interpreter",
	"powershell.exe": "shell interpreter",
	"pwsh":           "shell interpreter",
	"pwsh.exe":       "shell interpreter",
	"rm":             "destructive file command",
	"rmdir":          "destructive file command",
	"del":            "destructive file command",
	"erase":          "destructive file command",
	"dd":             "raw disk command",
	"mkfs":           "raw disk command",
	"mkfs.ext4":      "raw disk command",
	"diskutil":       "disk management command",
	"shutdown":       "system power command",
	"reboot":         "system power command",
	"halt":           "system power command",
	"poweroff":       "system power command",
}

func ValidateCommandSafety(command string, options CommandSafetyOptions) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("command is required")
	}
	if strings.ContainsAny(command, "\x00\r\n") {
		return errors.New("command must not contain control characters")
	}
	if strings.ContainsAny(command, "|&;<>`$") {
		return errors.New("command must be a single executable path or name; move shell syntax into explicit user-managed scripts")
	}
	if strings.ContainsAny(command, "\"'") {
		return errors.New("command must not be quoted; store the executable path without shell quoting")
	}
	if commandHasWhitespace(command) && !pathExists(command) {
		return errors.New("command must be a single executable path or name; put flags and prompt placeholders in args")
	}

	name := commandPolicyName(command)
	if reason, ok := riskyCommandReasons[name]; ok && !options.AllowRiskyCommand {
		return fmt.Errorf("command %q is blocked by default because it is a %s; set allow_risky_command only for an explicit user-managed CLI", name, reason)
	}
	return nil
}

func NormalizeEnvAllowlist(values []string) ([]string, error) {
	if values == nil {
		values = []string{}
	}
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		name := strings.TrimSpace(value)
		if err := ValidateEnvName(name); err != nil {
			return nil, err
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized, nil
}

func ResolveAllowedEnvironment(names []string) (map[string]string, error) {
	normalized, err := NormalizeEnvAllowlist(names)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, name := range normalized {
		if value, ok := os.LookupEnv(name); ok {
			env[name] = value
		}
	}
	return env, nil
}

func NormalizeCLIEnvironment(command string, env map[string]string) map[string]string {
	out := make(map[string]string, len(env)+8)
	for key, value := range env {
		out[key] = value
	}
	commandName := commandPolicyName(command)
	switch commandName {
	case "claude", "codex", "kiro", "kiro-cli", "opencode":
		ensureCLIRuntimeIdentity(out)
		out["PATH"] = normalizeCLIPath(out["PATH"], out["HOME"])
		if term := strings.TrimSpace(out["TERM"]); term == "" || term == "dumb" {
			out["TERM"] = "xterm-256color"
		}
		out["COLORTERM"] = "truecolor"
		out["NO_COLOR"] = "1"
		delete(out, "FORCE_COLOR")
		if developerDir := defaultDeveloperDir(); developerDir != "" && strings.TrimSpace(out["DEVELOPER_DIR"]) == "" {
			out["DEVELOPER_DIR"] = developerDir
		}
		if commandName == "codex" {
			out["RUST_LOG"] = "off"
		}
	case "gemini":
		ensureCLIRuntimeIdentity(out)
		out["PATH"] = normalizeCLIPath(out["PATH"], out["HOME"])
		if term := strings.TrimSpace(out["TERM"]); term == "" || term == "dumb" {
			out["TERM"] = "xterm-256color"
		}
		out["COLORTERM"] = "truecolor"
		out["NO_COLOR"] = "1"
		delete(out, "FORCE_COLOR")
		if developerDir := defaultDeveloperDir(); developerDir != "" && strings.TrimSpace(out["DEVELOPER_DIR"]) == "" {
			out["DEVELOPER_DIR"] = developerDir
		}
		out["NODE_OPTIONS"] = appendNodeOption(out["NODE_OPTIONS"], "--no-deprecation")
		out["GEMINI_PTY_INFO"] = "child_process"
	}
	return out
}

func PrepareCommandRuntimeEnvironment(command string, env map[string]string) (map[string]string, func(), error) {
	normalized := NormalizeCLIEnvironment(command, env)
	runtimeEnv, cleanupRuntime, err := prepareCLIToolRuntimeEnvironment(command, normalized)
	if err != nil {
		return nil, nil, err
	}
	if commandPolicyName(command) != "gemini" {
		return runtimeEnv, cleanupRuntime, nil
	}

	prepared, cleanupGemini, err := prepareGeminiRuntimeEnvironment(runtimeEnv)
	if err != nil {
		cleanupRuntime()
		return nil, nil, err
	}
	return prepared, cleanupAll(cleanupRuntime, cleanupGemini), nil
}

func ResolveCommandExecutable(command string) (string, error) {
	return ResolveCommandExecutableWithEnv(command, nil)
}

func ResolveCommandExecutableWithEnv(command string, env map[string]string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command is required")
	}
	if pathExists(command) {
		return command, nil
	}
	searchPath := ""
	if env != nil {
		searchPath = env["PATH"]
	}
	if env == nil && strings.TrimSpace(searchPath) == "" {
		searchPath = os.Getenv("PATH")
	}
	home := ""
	if env != nil {
		home = env["HOME"]
	}
	searchPath = normalizeCLIPath(searchPath, home)
	if resolved := lookPathIn(command, searchPath); resolved != "" {
		return resolved, nil
	}
	if env == nil {
		if resolved, err := exec.LookPath(command); err == nil {
			return resolved, nil
		}
	}
	if commandPolicyName(command) == "opencode" {
		if resolved := resolveOpenCodePackagedExecutable(env); resolved != "" {
			return resolved, nil
		}
	}
	return "", exec.ErrNotFound
}

func CLIRuntimeBaseDir() string {
	if base := strings.TrimSpace(os.Getenv("COCODE_AGENT_RUNTIME_DIR")); base != "" {
		return base
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "cocode-agent-runtime")
	}
	return "/tmp/cocode-agent-runtime"
}

func prepareCLIToolRuntimeEnvironment(command string, env map[string]string) (map[string]string, func(), error) {
	commandName := commandPolicyName(command)
	switch commandName {
	case "claude", "codex", "gemini", "kiro", "kiro-cli", "opencode":
	default:
		return env, func() {}, nil
	}
	base := CLIRuntimeBaseDir()
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create CLI runtime base: %w", err)
	}
	runDir, err := os.MkdirTemp(base, commandName+"-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create CLI runtime dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(runDir)
	}
	cacheDir := filepath.Join(runDir, "cache")
	goCacheDir := filepath.Join(cacheDir, "go-build")
	goplsCacheDir := filepath.Join(cacheDir, "gopls")
	for _, dir := range []string{cacheDir, goCacheDir, goplsCacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("create CLI cache dir: %w", err)
		}
	}
	out := make(map[string]string, len(env)+12)
	for key, value := range env {
		out[key] = value
	}
	out["TMPDIR"] = runDir
	out["TMP"] = runDir
	out["TEMP"] = runDir
	out["XDG_CACHE_HOME"] = cacheDir
	out["GOCACHE"] = goCacheDir
	out["GOPLSCACHE"] = goplsCacheDir
	if strings.TrimSpace(out["GOTOOLCHAIN"]) == "" {
		out["GOTOOLCHAIN"] = "auto"
	}
	if home := strings.TrimSpace(out["HOME"]); home != "" && strings.TrimSpace(out["GOENV_ROOT"]) == "" {
		goenvRoot := filepath.Join(home, ".goenv")
		if info, err := os.Stat(goenvRoot); err == nil && info.IsDir() {
			out["GOENV_ROOT"] = goenvRoot
		}
	}
	if runtime.GOOS == "darwin" {
		out["DARWIN_USER_TEMP_DIR"] = runDir
		out["DARWIN_USER_CACHE_DIR"] = cacheDir
	}
	return out, cleanup, nil
}

func cleanupAll(cleanups ...func()) func() {
	return func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				cleanups[i]()
			}
		}
	}
}

const geminiIsolatedContextFileName = "COCODE_EMPTY_CONTEXT.md"

var geminiCredentialFiles = []string{
	"oauth_creds.json",
	"google_accounts.json",
	"state.json",
	"projects.json",
	"trustedFolders.json",
	"installation_id",
	"gemini-credentials.json",
}

func prepareGeminiRuntimeEnvironment(env map[string]string) (map[string]string, func(), error) {
	sourceHome, err := sourceHomeDirectory(env)
	if err != nil {
		return nil, nil, err
	}
	sourceGeminiDir := filepath.Join(sourceHome, ".gemini")
	tempHome, err := os.MkdirTemp("", "cocode-gemini-home-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create isolated Gemini home: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempHome)
	}
	targetGeminiDir := filepath.Join(tempHome, ".gemini")
	if err := os.MkdirAll(targetGeminiDir, 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("create isolated Gemini config dir: %w", err)
	}

	for _, name := range geminiCredentialFiles {
		if err := copyFileIfExists(filepath.Join(sourceGeminiDir, name), filepath.Join(targetGeminiDir, name)); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	if err := writeIsolatedGeminiSettings(filepath.Join(sourceGeminiDir, "settings.json"), filepath.Join(targetGeminiDir, "settings.json")); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := os.WriteFile(filepath.Join(targetGeminiDir, geminiIsolatedContextFileName), nil, 0o600); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write isolated Gemini context file: %w", err)
	}

	prepared := make(map[string]string, len(env)+1)
	for key, value := range env {
		prepared[key] = value
	}
	prepared["HOME"] = tempHome
	return prepared, cleanup, nil
}

func sourceHomeDirectory(env map[string]string) (string, error) {
	if home := strings.TrimSpace(env["HOME"]); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for Gemini credentials: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("resolve user home for Gemini credentials: empty home directory")
	}
	return home, nil
}

func copyFileIfExists(source string, target string) error {
	in, err := os.Open(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open Gemini config %s: %w", filepath.Base(source), err)
	}
	defer func() {
		_ = in.Close()
	}()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("inspect Gemini config %s: %w", filepath.Base(source), err)
	}
	if info.IsDir() {
		return nil
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create isolated Gemini config %s: %w", filepath.Base(target), err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy Gemini config %s: %w", filepath.Base(source), err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close isolated Gemini config %s: %w", filepath.Base(target), err)
	}
	return nil
}

func writeIsolatedGeminiSettings(source string, target string) error {
	settings := map[string]any{}
	raw, err := os.ReadFile(source)
	switch {
	case err == nil && len(strings.TrimSpace(string(raw))) > 0:
		if err := json.Unmarshal(raw, &settings); err != nil {
			return fmt.Errorf("decode Gemini settings: %w", err)
		}
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("read Gemini settings: %w", err)
	}

	contextSettings, _ := settings["context"].(map[string]any)
	if contextSettings == nil {
		contextSettings = map[string]any{}
	}
	contextSettings["fileName"] = geminiIsolatedContextFileName
	contextSettings["includeDirectoryTree"] = false
	contextSettings["memoryBoundaryMarkers"] = []any{".git"}
	settings["context"] = contextSettings

	experimental, _ := settings["experimental"].(map[string]any)
	if experimental == nil {
		experimental = map[string]any{}
	}
	experimental["jitContext"] = false
	settings["experimental"] = experimental

	tools, _ := settings["tools"].(map[string]any)
	if tools == nil {
		tools = map[string]any{}
	}
	tools["useRipgrep"] = false
	shell, _ := tools["shell"].(map[string]any)
	if shell == nil {
		shell = map[string]any{}
	}
	shell["enableInteractiveShell"] = false
	shell["showColor"] = false
	tools["shell"] = shell
	settings["tools"] = tools

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode isolated Gemini settings: %w", err)
	}
	if err := os.WriteFile(target, append(out, '\n'), 0o600); err != nil {
		return fmt.Errorf("write isolated Gemini settings: %w", err)
	}
	return nil
}

func resolveOpenCodePackagedExecutable(env map[string]string) string {
	home := ""
	searchPath := ""
	pnpmHome := ""
	if env != nil {
		home = strings.TrimSpace(env["HOME"])
		searchPath = strings.TrimSpace(env["PATH"])
		pnpmHome = strings.TrimSpace(env["PNPM_HOME"])
	}
	if home == "" {
		home = strings.TrimSpace(os.Getenv("HOME"))
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if env == nil && searchPath == "" {
		searchPath = os.Getenv("PATH")
	}
	if env == nil && pnpmHome == "" {
		pnpmHome = os.Getenv("PNPM_HOME")
	}
	searchPath = normalizeCLIPath(searchPath, home)
	platformPackage := opencodePlatformPackage()
	patterns := []string{}
	for _, root := range opencodeInstallRoots(home, searchPath, pnpmHome) {
		pnpmDir := filepath.Join(root, "global", "*", ".pnpm")
		patterns = append(patterns, filepath.Join(pnpmDir, platformPackage+"@*", "node_modules", platformPackage, "bin", "opencode"))
		for _, packageName := range opencodePackageNames() {
			patterns = append(patterns, opencodeNodePackagePatterns(filepath.Join(pnpmDir, packageName+"@*", "node_modules", packageName), platformPackage)...)
		}
	}
	if home != "" {
		for _, packageName := range opencodePackageNames() {
			patterns = append(patterns, opencodeNodePackagePatterns(filepath.Join(home, ".npm-global", "lib", "node_modules", packageName), platformPackage)...)
			patterns = append(patterns, opencodeNodePackagePatterns(filepath.Join(home, ".nvm", "versions", "node", "*", "lib", "node_modules", packageName), platformPackage)...)
			patterns = append(patterns, opencodeNodePackagePatterns(filepath.Join(home, ".local", "share", "fnm", "node-versions", "*", "installation", "lib", "node_modules", packageName), platformPackage)...)
			patterns = append(patterns, opencodeNodePackagePatterns(filepath.Join(home, ".bun", "install", "global", "node_modules", packageName), platformPackage)...)
		}
	}
	if runtime.GOOS == "darwin" {
		for _, packageName := range opencodePackageNames() {
			patterns = append(patterns, opencodeNodePackagePatterns(filepath.Join("/opt/homebrew", "lib", "node_modules", packageName), platformPackage)...)
			patterns = append(patterns, opencodeNodePackagePatterns(filepath.Join("/usr/local", "lib", "node_modules", packageName), platformPackage)...)
		}
	}
	for _, pattern := range patterns {
		matches := []string{pattern}
		if strings.Contains(pattern, "*") {
			var err error
			matches, err = filepath.Glob(pattern)
			if err != nil {
				continue
			}
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		}
		for _, candidate := range matches {
			if executableFile(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func opencodePackageNames() []string {
	return []string{"opencode", "opencode-ai"}
}

func opencodeNodePackagePatterns(packageRoot string, platformPackage string) []string {
	return []string{
		filepath.Join(packageRoot, "node_modules", platformPackage, "bin", "opencode"),
		filepath.Join(packageRoot, "bin", "opencode"),
	}
}

func opencodeInstallRoots(home string, searchPath string, pnpmHome string) []string {
	roots := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, root := range roots {
			if root == value {
				return
			}
		}
		roots = append(roots, value)
	}
	if pnpmHome = strings.TrimSpace(pnpmHome); pnpmHome != "" {
		add(pnpmHome)
	}
	if home != "" {
		add(filepath.Join(home, "Library", "pnpm"))
		add(filepath.Join(home, ".local", "share", "pnpm"))
		add(filepath.Join(home, ".pnpm"))
	}
	for _, dir := range filepath.SplitList(searchPath) {
		if filepath.Base(dir) == "pnpm" {
			add(dir)
		}
	}
	return roots
}

func opencodePlatformPackage() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return "opencode-" + runtime.GOOS + "-" + arch
}

func ValidateEnvName(name string) error {
	if name == "" {
		return errors.New("environment variable name cannot be empty")
	}
	if strings.TrimSpace(name) != name || strings.Contains(name, "=") {
		return fmt.Errorf("invalid environment variable name %q", name)
	}
	for index, r := range name {
		if index == 0 {
			if !isEnvNameStart(r) {
				return fmt.Errorf("invalid environment variable name %q", name)
			}
			continue
		}
		if !isEnvNamePart(r) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
	}
	return nil
}

func envList(env map[string]string) []string {
	if len(env) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func validateEnv(env map[string]string) error {
	for key := range env {
		if err := ValidateEnvName(key); err != nil {
			return err
		}
	}
	return nil
}

func commandPolicyName(command string) string {
	target := command
	if commandHasWhitespace(command) && !pathExists(command) {
		fields := strings.Fields(command)
		if len(fields) > 0 {
			target = fields[0]
		}
	}
	return strings.ToLower(filepath.Base(target))
}

func commandHasWhitespace(command string) bool {
	return strings.IndexFunc(command, unicode.IsSpace) >= 0
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func defaultDeveloperDir() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	const commandLineToolsDir = "/Library/Developer/CommandLineTools"
	if info, err := os.Stat(commandLineToolsDir); err == nil && info.IsDir() {
		return commandLineToolsDir
	}
	return ""
}

func ensureCLIRuntimeIdentity(env map[string]string) {
	if env == nil {
		return
	}
	if strings.TrimSpace(env["HOME"]) == "" {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			env["HOME"] = home
		}
	}
	userName := firstNonEmpty(os.Getenv("USER"), os.Getenv("LOGNAME"), currentUserName(), filepath.Base(strings.TrimSpace(env["HOME"])))
	if strings.TrimSpace(env["USER"]) == "" && userName != "" {
		env["USER"] = userName
	}
	if strings.TrimSpace(env["LOGNAME"]) == "" {
		env["LOGNAME"] = firstNonEmpty(os.Getenv("LOGNAME"), env["USER"], userName)
	}
	if strings.TrimSpace(env["SHELL"]) == "" {
		env["SHELL"] = firstExistingFile(os.Getenv("SHELL"), "/bin/zsh", "/bin/bash", "/bin/sh")
	}
	if strings.TrimSpace(env["TMPDIR"]) == "" {
		env["TMPDIR"] = os.TempDir()
	}
}

func currentUserName() string {
	current, err := osuser.Current()
	if err != nil || current == nil {
		return ""
	}
	username := strings.TrimSpace(current.Username)
	if username == "" {
		return ""
	}
	if slash := strings.LastIndexAny(username, `\/`); slash >= 0 && slash+1 < len(username) {
		username = username[slash+1:]
	}
	return username
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstExistingFile(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if info, err := os.Stat(value); err == nil && !info.IsDir() {
			return value
		}
	}
	return ""
}

func normalizeCLIPath(value string, home string) string {
	home = strings.TrimSpace(home)
	parts := make([]string, 0, len(filepath.SplitList(value))+24)
	seen := map[string]struct{}{}
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		parts = append(parts, dir)
	}
	if home != "" {
		addPreferredGoToolDirs(add, home)
	}
	for _, dir := range filepath.SplitList(value) {
		add(dir)
	}
	if home != "" {
		for _, dir := range []string{
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".npm-global", "bin"),
			filepath.Join(home, "Library", "pnpm"),
			filepath.Join(home, ".local", "share", "pnpm"),
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".yarn", "bin"),
			filepath.Join(home, ".config", "yarn", "global", "node_modules", ".bin"),
			filepath.Join(home, ".deno", "bin"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, ".volta", "bin"),
			filepath.Join(home, ".asdf", "shims"),
			filepath.Join(home, ".nodenv", "shims"),
			filepath.Join(home, ".goenv", "shims"),
		} {
			addIfDirExists(add, dir)
		}
		addGlobDirs(add, filepath.Join(home, ".nvm", "versions", "node", "*", "bin"))
		addGlobDirs(add, filepath.Join(home, ".local", "share", "fnm", "node-versions", "*", "installation", "bin"))
	}
	if developerDir := defaultDeveloperDir(); developerDir != "" {
		addIfDirExists(add, filepath.Join(developerDir, "usr", "bin"))
	}
	if runtime.GOOS == "darwin" {
		for _, dir := range []string{
			"/opt/homebrew/bin",
			"/opt/homebrew/sbin",
			"/usr/local/bin",
			"/usr/local/sbin",
			"/System/Cryptexes/App/usr/bin",
			"/usr/bin",
			"/bin",
			"/usr/sbin",
			"/sbin",
			"/Library/Apple/usr/bin",
		} {
			addIfDirExists(add, dir)
		}
	} else {
		for _, dir := range []string{"/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
			addIfDirExists(add, dir)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func addPreferredGoToolDirs(add func(string), home string) {
	matches, err := filepath.Glob(filepath.Join(home, "go", "*", "bin"))
	if err == nil {
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		for _, match := range matches {
			addIfDirExists(add, match)
		}
	}
	addIfDirExists(add, filepath.Join(home, ".goenv", "shims"))
}

func addIfDirExists(add func(string), dir string) {
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		add(dir)
	}
}

func addGlobDirs(add func(string), pattern string) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	for _, match := range matches {
		addIfDirExists(add, match)
	}
}

func lookPathIn(command string, searchPath string) string {
	if strings.TrimSpace(command) == "" || strings.ContainsRune(command, os.PathSeparator) {
		return ""
	}
	for _, dir := range filepath.SplitList(searchPath) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, command)
		if executableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func prependPathIfDir(value string, dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return value
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return value
	}
	parts := strings.Split(value, string(os.PathListSeparator))
	for _, part := range parts {
		if part == dir {
			return value
		}
	}
	if strings.TrimSpace(value) == "" {
		return dir
	}
	return dir + string(os.PathListSeparator) + value
}

func appendNodeOption(value string, option string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return option
	}
	if strings.Contains(value, option) {
		return value
	}
	return value + " " + option
}

func isEnvNameStart(r rune) bool {
	return r == '_' || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

func isEnvNamePart(r rune) bool {
	return isEnvNameStart(r) || ('0' <= r && r <= '9')
}
