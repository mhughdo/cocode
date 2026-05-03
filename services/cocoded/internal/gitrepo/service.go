package gitrepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const gitTimeout = 5 * time.Second

type Service struct {
	database *sql.DB
	queries  *dbgen.Queries
	now      func() time.Time
}

type RepositoryInfo struct {
	SelectedPath  string
	RootPath      string
	Name          string
	Owner         string
	RemoteURL     string
	DefaultBranch string
}

type OpenResult struct {
	Info       RepositoryInfo
	Workspace  dbgen.Workspace
	Repository dbgen.Repository
}

func New(database *sql.DB) (*Service, error) {
	if database == nil {
		return nil, errors.New("git repository database is required")
	}
	return &Service{
		database: database,
		queries:  dbgen.New(database),
		now:      time.Now,
	}, nil
}

func (s *Service) Open(ctx context.Context, selectedPath string) (OpenResult, error) {
	if s == nil {
		return OpenResult{}, apperror.Internal("git repository service is not configured")
	}

	info, err := Validate(ctx, selectedPath)
	if err != nil {
		return OpenResult{}, err
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return OpenResult{}, apperror.Internal("failed to start repository setup")
	}
	defer tx.Rollback()

	queries := s.queries.WithTx(tx)
	now := s.now().UTC().Format(time.RFC3339Nano)

	workspace, err := getOrCreateWorkspace(ctx, queries, info, now)
	if err != nil {
		return OpenResult{}, err
	}
	repository, err := getOrCreateRepository(ctx, queries, workspace.ID, info, now)
	if err != nil {
		return OpenResult{}, err
	}
	workspace, err = queries.UpdateWorkspace(ctx, dbgen.UpdateWorkspaceParams{
		ID:            workspace.ID,
		Name:          workspace.Name,
		DefaultRepoID: sql.NullString{String: repository.ID, Valid: true},
		SettingsJson:  workspace.SettingsJson,
		UpdatedAt:     now,
	})
	if err != nil {
		return OpenResult{}, apperror.Internal("failed to update workspace repository")
	}

	if err := tx.Commit(); err != nil {
		return OpenResult{}, apperror.Internal("failed to save repository setup")
	}

	return OpenResult{
		Info:       info,
		Workspace:  workspace,
		Repository: repository,
	}, nil
}

func Validate(ctx context.Context, selectedPath string) (RepositoryInfo, error) {
	selectedPath = strings.TrimSpace(selectedPath)
	if selectedPath == "" {
		return RepositoryInfo{}, apperror.InvalidRequest("repository path is required")
	}

	absoluteSelected, err := filepath.Abs(selectedPath)
	if err != nil {
		return RepositoryInfo{}, apperror.InvalidRequest("repository path is invalid")
	}
	stat, err := os.Stat(absoluteSelected)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RepositoryInfo{}, apperror.InvalidRequest("repository path does not exist")
		}
		return RepositoryInfo{}, apperror.InvalidRequest("repository path cannot be inspected")
	}
	if !stat.IsDir() {
		return RepositoryInfo{}, apperror.InvalidRequest("repository path must be a directory")
	}

	root, err := runGit(ctx, absoluteSelected, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepositoryInfo{}, apperror.InvalidRequest("selected path is not inside a git repository")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return RepositoryInfo{}, apperror.InvalidRequest("selected path is not inside a git repository")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return RepositoryInfo{}, apperror.InvalidRequest("git repository root is invalid")
	}

	remoteURL := optionalGit(ctx, root, "config", "--get", "remote.origin.url")
	defaultBranch := optionalGit(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if defaultBranch == "" {
		defaultBranch = optionalGit(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	}
	if defaultBranch == "HEAD" {
		defaultBranch = ""
	}

	return RepositoryInfo{
		SelectedPath:  absoluteSelected,
		RootPath:      root,
		Name:          filepath.Base(root),
		Owner:         inferGitHubOwner(remoteURL),
		RemoteURL:     remoteURL,
		DefaultBranch: defaultBranch,
	}, nil
}

func getOrCreateWorkspace(ctx context.Context, queries *dbgen.Queries, info RepositoryInfo, now string) (dbgen.Workspace, error) {
	workspace, err := queries.GetWorkspaceByRootPath(ctx, info.RootPath)
	if err == nil {
		return workspace, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return dbgen.Workspace{}, apperror.Internal("failed to read workspace")
	}

	workspace, err = queries.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{
		ID:            stableID("workspace", info.RootPath),
		Name:          info.Name,
		RootPath:      info.RootPath,
		DefaultRepoID: sql.NullString{},
		SettingsJson:  `{"source":"git_repo_validation"}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return dbgen.Workspace{}, apperror.Internal("failed to create workspace")
	}
	return workspace, nil
}

func getOrCreateRepository(ctx context.Context, queries *dbgen.Queries, workspaceID string, info RepositoryInfo, now string) (dbgen.Repository, error) {
	repository, err := queries.GetRepositoryByLocalPath(ctx, dbgen.GetRepositoryByLocalPathParams{
		WorkspaceID: workspaceID,
		LocalPath:   info.RootPath,
	})
	if err == nil {
		repository, err = queries.UpdateRepository(ctx, dbgen.UpdateRepositoryParams{
			ID:            repository.ID,
			Name:          info.Name,
			Owner:         nullableString(info.Owner),
			RemoteUrl:     nullableString(info.RemoteURL),
			DefaultBranch: nullableString(info.DefaultBranch),
			UpdatedAt:     now,
		})
		if err != nil {
			return dbgen.Repository{}, apperror.Internal("failed to update repository")
		}
		return repository, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return dbgen.Repository{}, apperror.Internal("failed to read repository")
	}

	repository, err = queries.CreateRepository(ctx, dbgen.CreateRepositoryParams{
		ID:            stableID("repo", info.RootPath),
		WorkspaceID:   workspaceID,
		Name:          info.Name,
		Owner:         nullableString(info.Owner),
		RemoteUrl:     nullableString(info.RemoteURL),
		LocalPath:     info.RootPath,
		DefaultBranch: nullableString(info.DefaultBranch),
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return dbgen.Repository{}, apperror.Internal("failed to create repository")
	}
	return repository, nil
}

func runGit(ctx context.Context, cwd string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", err
	}

	runCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	commandArgs := append([]string{"-C", cwd}, args...)
	cmd := exec.CommandContext(runCtx, "git", commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.Output()
	if runCtx.Err() != nil {
		return "", runCtx.Err()
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func optionalGit(ctx context.Context, cwd string, args ...string) string {
	value, err := runGit(ctx, cwd, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func inferGitHubOwner(remoteURL string) string {
	remoteURL = strings.TrimSpace(strings.TrimSuffix(remoteURL, ".git"))
	if remoteURL == "" {
		return ""
	}
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		path := strings.TrimPrefix(remoteURL, "git@github.com:")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return parts[0]
		}
	}

	marker := "github.com/"
	index := strings.Index(remoteURL, marker)
	if index < 0 {
		return ""
	}
	path := remoteURL[index+len(marker):]
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

func stableID(prefix string, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(sum[:8])
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
