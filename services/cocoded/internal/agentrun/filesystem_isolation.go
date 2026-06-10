package agentrun

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
	"github.com/hughdo/cocode/services/cocoded/internal/gitrepo"
)

type FilesystemIsolationMode string

const (
	FilesystemIsolationAuto                 FilesystemIsolationMode = ""
	FilesystemIsolationNone                 FilesystemIsolationMode = "none"
	FilesystemIsolationEphemeralGitWorktree FilesystemIsolationMode = "ephemeral_git_worktree"
)

type FilesystemIsolationOptions struct {
	Mode    FilesystemIsolationMode
	TempDir string
}

type preparedFilesystemIsolation struct {
	mode       FilesystemIsolationMode
	worktree   gitrepo.EphemeralWorktree
	config     agents.ConnectionConfig
	task       agents.AgentTask
	metadata   map[string]any
	cleanupRun bool
}

func (r Runner) prepareFilesystemIsolation(ctx context.Context, params RunParams, config agents.ConnectionConfig, task agents.AgentTask) (preparedFilesystemIsolation, error) {
	mode := params.Filesystem.modeFor(params.Permissions, config, params.Capabilities, task)
	prepared := preparedFilesystemIsolation{
		mode:   mode,
		config: config,
		task:   task,
	}
	if mode == FilesystemIsolationNone {
		return prepared, nil
	}
	if mode != FilesystemIsolationEphemeralGitWorktree {
		return prepared, fmt.Errorf("filesystem isolation mode %q is invalid", mode)
	}

	sourceRoot := firstNonEmpty(task.RepositoryRoot, config.WorkingDirectory)
	if strings.TrimSpace(sourceRoot) == "" {
		return prepared, nil
	}
	runner := r.GitRunner
	if runner == (gitrepo.Runner{}) {
		runner = gitrepo.DefaultRunner()
	}
	worktree, err := runner.CreateEphemeralWorktree(ctx, sourceRoot, params.Filesystem.TempDir)
	if err != nil {
		return prepared, err
	}

	prepared.worktree = worktree
	prepared.cleanupRun = true
	prepared.config.WorkingDirectory = remapIsolatedPath(config.WorkingDirectory, worktree.SourceRoot, worktree.Path, worktree.Path)
	prepared.task.RepositoryRoot = worktree.Path
	prepared.task.WorkspaceRoot = remapIsolatedPath(task.WorkspaceRoot, worktree.SourceRoot, worktree.Path, worktree.Path)
	prepared.metadata = map[string]any{
		"mode":          string(mode),
		"source_root":   worktree.SourceRoot,
		"isolated_root": worktree.Path,
	}
	if prepared.task.Metadata == nil {
		prepared.task.Metadata = map[string]any{}
	}
	prepared.task.Metadata["execution_isolation"] = prepared.metadata
	return prepared, nil
}

func (i *preparedFilesystemIsolation) Cleanup(ctx context.Context) error {
	if i == nil || !i.cleanupRun {
		return nil
	}
	i.cleanupRun = false
	return i.worktree.Cleanup(ctx)
}

func (i preparedFilesystemIsolation) event(runID string) agents.AgentEvent {
	return agents.AgentEvent{
		Type:    agents.EventProgress,
		RunID:   runID,
		Message: "ephemeral git worktree prepared for review run",
		Metadata: map[string]any{
			"execution_isolation": i.metadata,
		},
	}
}

func (o FilesystemIsolationOptions) modeFor(policy agents.PermissionPolicy, config agents.ConnectionConfig, capabilities agents.AgentCapabilities, task agents.AgentTask) FilesystemIsolationMode {
	if o.Mode == FilesystemIsolationNone {
		return FilesystemIsolationNone
	}
	if o.Mode != FilesystemIsolationAuto {
		return o.Mode
	}
	if policy.Mode != agents.PermissionModeReview {
		return FilesystemIsolationNone
	}
	if !filesystemBackedAdapter(config.Kind) {
		return FilesystemIsolationNone
	}
	if agentCapabilitiesEmpty(capabilities) {
		capabilities = agents.DefaultCapabilities(config.Kind)
	}
	if !capabilities.CanRead {
		return FilesystemIsolationNone
	}
	if strings.TrimSpace(task.RepositoryRoot) == "" && strings.TrimSpace(config.WorkingDirectory) == "" {
		return FilesystemIsolationNone
	}
	return FilesystemIsolationEphemeralGitWorktree
}

func agentCapabilitiesEmpty(capabilities agents.AgentCapabilities) bool {
	return !capabilities.SupportsJSON &&
		!capabilities.SupportsStreaming &&
		!capabilities.SupportsSessions &&
		!capabilities.CanRead &&
		!capabilities.CanWrite &&
		!capabilities.CanCancel &&
		len(capabilities.OutputModes) == 0 &&
		len(capabilities.Metadata) == 0
}

func filesystemBackedAdapter(kind agents.AdapterKind) bool {
	switch kind {
	case agents.AdapterCLINonInteractive, agents.AdapterJSONRPCStdio, agents.AdapterACPStdio:
		return true
	default:
		return false
	}
}

func remapIsolatedPath(path string, sourceRoot string, isolatedRoot string, fallback string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return fallback
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fallback
	}
	absSource, err := filepath.Abs(sourceRoot)
	if err != nil {
		return fallback
	}
	rel, err := filepath.Rel(absSource, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fallback
	}
	if rel == "." {
		return isolatedRoot
	}
	return filepath.Join(isolatedRoot, rel)
}
