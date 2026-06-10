package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

const defaultEphemeralWorktreePrefix = "cocode-review-worktree-"

type EphemeralWorktree struct {
	Path       string
	SourceRoot string

	runner Runner
	parent string
}

func (r Runner) CreateEphemeralWorktree(ctx context.Context, repoPath string, tempDir string) (EphemeralWorktree, error) {
	ctx = contextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return EphemeralWorktree{}, err
	}
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return EphemeralWorktree{}, apperror.InvalidRequest("ephemeral worktree repository path is required")
	}
	source, err := r.Run(ctx, repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return EphemeralWorktree{}, fmt.Errorf("resolve git worktree source root: %w", err)
	}
	sourceRoot, err := filepath.Abs(source.Stdout)
	if err != nil {
		return EphemeralWorktree{}, fmt.Errorf("resolve git worktree source root path: %w", err)
	}
	if _, err := r.Run(ctx, sourceRoot, "rev-parse", "--verify", "HEAD"); err != nil {
		return EphemeralWorktree{}, fmt.Errorf("resolve git worktree HEAD: %w", err)
	}

	parent, err := os.MkdirTemp(tempDir, defaultEphemeralWorktreePrefix)
	if err != nil {
		return EphemeralWorktree{}, fmt.Errorf("create ephemeral worktree temp dir: %w", err)
	}
	path := filepath.Join(parent, "repo")
	if _, err := r.Run(ctx, sourceRoot, "worktree", "add", "--detach", path, "HEAD"); err != nil {
		_ = os.RemoveAll(parent)
		return EphemeralWorktree{}, fmt.Errorf("create ephemeral git worktree: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		_ = r.removeWorktree(context.WithoutCancel(ctx), sourceRoot, path)
		_ = os.RemoveAll(parent)
		return EphemeralWorktree{}, fmt.Errorf("resolve ephemeral worktree path: %w", err)
	}
	return EphemeralWorktree{
		Path:       absPath,
		SourceRoot: sourceRoot,
		runner:     r,
		parent:     parent,
	}, nil
}

func (w EphemeralWorktree) Cleanup(ctx context.Context) error {
	if strings.TrimSpace(w.Path) == "" {
		return nil
	}
	ctx = contextOrBackground(ctx)
	sourceRoot := strings.TrimSpace(w.SourceRoot)
	if sourceRoot == "" {
		sourceRoot = w.Path
	}
	var errs []error
	if err := w.runner.removeWorktree(ctx, sourceRoot, w.Path); err != nil {
		errs = append(errs, err)
	}
	removePath := w.parent
	if strings.TrimSpace(removePath) == "" {
		removePath = w.Path
	}
	if err := os.RemoveAll(removePath); err != nil {
		errs = append(errs, fmt.Errorf("remove ephemeral worktree files: %w", err))
	}
	return errors.Join(errs...)
}

func (r Runner) removeWorktree(ctx context.Context, sourceRoot string, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := r.Run(ctx, sourceRoot, "worktree", "remove", "--force", path); err != nil {
		return fmt.Errorf("remove ephemeral git worktree: %w", err)
	}
	return nil
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
