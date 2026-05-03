package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func isEnvNameStart(r rune) bool {
	return r == '_' || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

func isEnvNamePart(r rune) bool {
	return isEnvNameStart(r) || ('0' <= r && r <= '9')
}
