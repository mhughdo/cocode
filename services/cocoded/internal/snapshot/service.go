package snapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/artifact"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/githubpr"
)

type Service struct {
	queries   *dbgen.Queries
	artifacts *artifact.Store
	now       func() time.Time
}

type GitHubSnapshotParams struct {
	WorkspaceID  string
	RepositoryID string
	Metadata     githubpr.Metadata
	Files        []githubpr.ChangedFile
	Diff         []byte
}

type GitHubSnapshotResult struct {
	Snapshot       dbgen.PullRequestSnapshot
	ChangedFiles   []dbgen.ChangedFile
	DiffArtifact   dbgen.Artifact
	PatchArtifacts []dbgen.Artifact
}

func New(database *sql.DB, artifactRoot string) (*Service, error) {
	if database == nil {
		return nil, errors.New("snapshot database is required")
	}
	queries := dbgen.New(database)
	artifactStore, err := artifact.New(artifactRoot, queries)
	if err != nil {
		return nil, err
	}
	return &Service{
		queries:   queries,
		artifacts: artifactStore,
		now:       time.Now,
	}, nil
}

func (s *Service) CreateGitHubSnapshot(ctx context.Context, params GitHubSnapshotParams) (GitHubSnapshotResult, error) {
	if s == nil {
		return GitHubSnapshotResult{}, errors.New("snapshot service is required")
	}
	if params.WorkspaceID == "" {
		return GitHubSnapshotResult{}, errors.New("workspace id is required")
	}
	if params.RepositoryID == "" {
		return GitHubSnapshotResult{}, errors.New("repository id is required")
	}
	if params.Metadata.Owner == "" || params.Metadata.Repo == "" || params.Metadata.Number <= 0 {
		return GitHubSnapshotResult{}, errors.New("GitHub pull request metadata is required")
	}
	if err := s.validateGitHubSnapshotInput(ctx, params); err != nil {
		return GitHubSnapshotResult{}, err
	}

	createdAt := s.now().UTC().Format(time.RFC3339Nano)
	snapshotID := stableID("snapshot", strings.Join([]string{
		params.RepositoryID,
		params.Metadata.Owner,
		params.Metadata.Repo,
		strconv.FormatInt(params.Metadata.Number, 10),
		params.Metadata.HeadSHA,
		createdAt,
	}, "\x00"))

	diffArtifact, err := s.artifacts.Save(ctx, artifact.SaveParams{
		ID:           stableID("artifact", snapshotID+"\x00diff"),
		WorkspaceID:  params.WorkspaceID,
		Kind:         "diff",
		RelativePath: filepath.ToSlash(filepath.Join("snapshots", snapshotID, "diff.patch")),
		ContentType:  "text/x-diff",
		MetadataJSON: mustJSON(map[string]any{
			"source":    "github",
			"owner":     params.Metadata.Owner,
			"repo":      params.Metadata.Repo,
			"pr_number": params.Metadata.Number,
			"head_sha":  params.Metadata.HeadSHA,
		}),
		CreatedAt: createdAt,
	}, params.Diff)
	if err != nil {
		return GitHubSnapshotResult{}, fmt.Errorf("save diff artifact: %w", err)
	}

	snapshot, err := s.queries.CreatePullRequestSnapshot(ctx, dbgen.CreatePullRequestSnapshotParams{
		ID:             snapshotID,
		RepositoryID:   params.RepositoryID,
		SourceType:     "github_pr",
		Provider:       nullableString("github"),
		Owner:          nullableString(params.Metadata.Owner),
		Repo:           nullableString(params.Metadata.Repo),
		PrNumber:       sql.NullInt64{Int64: params.Metadata.Number, Valid: true},
		PrTitle:        nullableString(params.Metadata.Title),
		PrUrl:          nullableString(params.Metadata.URL),
		BaseRef:        nullableString(params.Metadata.BaseRef),
		HeadRef:        nullableString(params.Metadata.HeadRef),
		BaseSha:        nullableString(params.Metadata.BaseSHA),
		HeadSha:        nullableString(params.Metadata.HeadSHA),
		DiffArtifactID: nullableString(diffArtifact.ID),
		MetadataJson: mustJSON(map[string]any{
			"source":      "github",
			"file_count":  len(params.Files),
			"diff_sha256": diffArtifact.Sha256.String,
		}),
		CreatedAt: createdAt,
	})
	if err != nil {
		return GitHubSnapshotResult{}, fmt.Errorf("create pull request snapshot: %w", err)
	}

	result := GitHubSnapshotResult{
		Snapshot:     snapshot,
		DiffArtifact: diffArtifact,
	}
	for index, file := range params.Files {
		changedFile, patchArtifact, err := s.createChangedFile(ctx, params.WorkspaceID, snapshotID, file, index, createdAt)
		if err != nil {
			return GitHubSnapshotResult{}, err
		}
		result.ChangedFiles = append(result.ChangedFiles, changedFile)
		if patchArtifact.ID != "" {
			result.PatchArtifacts = append(result.PatchArtifacts, patchArtifact)
		}
	}

	return result, nil
}

func (s *Service) validateGitHubSnapshotInput(ctx context.Context, params GitHubSnapshotParams) error {
	workspace, err := s.queries.GetWorkspace(ctx, params.WorkspaceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("workspace was not found")
		}
		return fmt.Errorf("read workspace: %w", err)
	}
	repository, err := s.queries.GetRepository(ctx, params.RepositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("repository was not found")
		}
		return fmt.Errorf("read repository: %w", err)
	}
	if repository.WorkspaceID != workspace.ID {
		return errors.New("repository does not belong to workspace")
	}

	paths := map[string]struct{}{}
	for _, file := range params.Files {
		path := strings.TrimSpace(file.Filename)
		if path == "" {
			return errors.New("changed file path is required")
		}
		if _, exists := paths[path]; exists {
			return fmt.Errorf("duplicate changed file path %q", path)
		}
		paths[path] = struct{}{}
	}
	return nil
}

func (s *Service) createChangedFile(ctx context.Context, workspaceID string, snapshotID string, file githubpr.ChangedFile, index int, createdAt string) (dbgen.ChangedFile, dbgen.Artifact, error) {
	var patchArtifact dbgen.Artifact
	if file.Patch != "" {
		artifactID := stableID("artifact", snapshotID+"\x00patch\x00"+file.Filename)
		relativePath := filepath.ToSlash(filepath.Join(
			"snapshots",
			snapshotID,
			"patches",
			fmt.Sprintf("%03d-%s.patch", index+1, safeArtifactName(file.Filename)),
		))
		saved, err := s.artifacts.Save(ctx, artifact.SaveParams{
			ID:           artifactID,
			WorkspaceID:  workspaceID,
			Kind:         "patch",
			RelativePath: relativePath,
			ContentType:  "text/x-diff",
			MetadataJSON: mustJSON(map[string]any{
				"source":            "github",
				"path":              file.Filename,
				"previous_filename": file.PreviousFilename,
				"status":            file.Status,
				"sha":               file.SHA,
			}),
			CreatedAt: createdAt,
		}, []byte(file.Patch))
		if err != nil {
			return dbgen.ChangedFile{}, dbgen.Artifact{}, fmt.Errorf("save patch artifact for %s: %w", file.Filename, err)
		}
		patchArtifact = saved
	}

	changedFile, err := s.queries.CreateChangedFile(ctx, dbgen.CreateChangedFileParams{
		ID:              stableID("file", snapshotID+"\x00"+file.Filename),
		SnapshotID:      snapshotID,
		Path:            file.Filename,
		OldPath:         nullableString(file.PreviousFilename),
		Status:          normalizeStatus(file.Status),
		Additions:       file.Additions,
		Deletions:       file.Deletions,
		IsBinary:        0,
		IsGenerated:     0,
		IsExcluded:      0,
		LineRangesJson:  "[]",
		PatchArtifactID: nullableString(patchArtifact.ID),
		CreatedAt:       createdAt,
	})
	if err != nil {
		return dbgen.ChangedFile{}, dbgen.Artifact{}, fmt.Errorf("create changed file %s: %w", file.Filename, err)
	}

	return changedFile, patchArtifact, nil
}

func normalizeStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case "added", "removed", "modified", "renamed", "copied", "changed", "unchanged":
		return status
	default:
		if status == "" {
			return "modified"
		}
		return status
	}
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

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

var unsafeArtifactName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeArtifactName(path string) string {
	name := strings.Trim(filepath.Base(path), ".")
	name = unsafeArtifactName.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "patch"
	}
	return name
}
