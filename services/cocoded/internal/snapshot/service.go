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
	"github.com/hughdo/cocode/services/cocoded/internal/fileclassify"
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

type GitSnapshotParams struct {
	WorkspaceID  string
	RepositoryID string
	SourceType   string
	BaseRef      string
	HeadRef      string
	BaseSHA      string
	HeadSHA      string
	Metadata     map[string]any
	Files        []ChangedFileInput
	Diff         []byte
}

type ChangedFileInput struct {
	Path           string
	OldPath        string
	Status         string
	Additions      int64
	Deletions      int64
	IsBinary       bool
	IsGenerated    bool
	IsExcluded     bool
	LineRangesJSON string
	Patch          string
	Metadata       map[string]any
}

type SnapshotResult struct {
	Snapshot       dbgen.PullRequestSnapshot
	ChangedFiles   []dbgen.ChangedFile
	DiffArtifact   dbgen.Artifact
	PatchArtifacts []dbgen.Artifact
}

type GitHubSnapshotResult = SnapshotResult

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

	files := make([]ChangedFileInput, 0, len(params.Files))
	for _, file := range params.Files {
		files = append(files, ChangedFileInput{
			Path:      file.Filename,
			OldPath:   file.PreviousFilename,
			Status:    file.Status,
			Additions: file.Additions,
			Deletions: file.Deletions,
			Patch:     file.Patch,
			Metadata: map[string]any{
				"sha":      file.SHA,
				"blob_url": file.BlobURL,
				"raw_url":  file.RawURL,
			},
		})
	}

	return s.createSnapshot(ctx, snapshotInput{
		WorkspaceID:  params.WorkspaceID,
		RepositoryID: params.RepositoryID,
		SourceType:   "github_pr",
		Provider:     "github",
		Owner:        params.Metadata.Owner,
		Repo:         params.Metadata.Repo,
		PRNumber:     params.Metadata.Number,
		PRTitle:      params.Metadata.Title,
		PRURL:        params.Metadata.URL,
		BaseRef:      params.Metadata.BaseRef,
		HeadRef:      params.Metadata.HeadRef,
		BaseSHA:      params.Metadata.BaseSHA,
		HeadSHA:      params.Metadata.HeadSHA,
		Metadata: map[string]any{
			"source":    "github",
			"owner":     params.Metadata.Owner,
			"repo":      params.Metadata.Repo,
			"pr_number": params.Metadata.Number,
			"head_sha":  params.Metadata.HeadSHA,
		},
		Files: files,
		Diff:  params.Diff,
	})
}

func (s *Service) CreateGitSnapshot(ctx context.Context, params GitSnapshotParams) (SnapshotResult, error) {
	if s == nil {
		return SnapshotResult{}, errors.New("snapshot service is required")
	}
	if params.SourceType != "branch_compare" && params.SourceType != "commit_compare" && params.SourceType != "local_changes" {
		return SnapshotResult{}, errors.New("git snapshot source type is invalid")
	}
	return s.createSnapshot(ctx, snapshotInput{
		WorkspaceID:  params.WorkspaceID,
		RepositoryID: params.RepositoryID,
		SourceType:   params.SourceType,
		BaseRef:      params.BaseRef,
		HeadRef:      params.HeadRef,
		BaseSHA:      params.BaseSHA,
		HeadSHA:      params.HeadSHA,
		Metadata:     params.Metadata,
		Files:        params.Files,
		Diff:         params.Diff,
	})
}

type snapshotInput struct {
	WorkspaceID  string
	RepositoryID string
	SourceType   string
	Provider     string
	Owner        string
	Repo         string
	PRNumber     int64
	PRTitle      string
	PRURL        string
	BaseRef      string
	HeadRef      string
	BaseSHA      string
	HeadSHA      string
	Metadata     map[string]any
	Files        []ChangedFileInput
	Diff         []byte
}

func (s *Service) createSnapshot(ctx context.Context, params snapshotInput) (SnapshotResult, error) {
	if err := s.validateSnapshotInput(ctx, params); err != nil {
		return SnapshotResult{}, err
	}

	createdAt := s.now().UTC().Format(time.RFC3339Nano)
	snapshotID := stableID("snapshot", strings.Join([]string{
		params.RepositoryID,
		params.SourceType,
		params.Owner,
		params.Repo,
		strconv.FormatInt(params.PRNumber, 10),
		params.BaseRef,
		params.HeadRef,
		params.BaseSHA,
		params.HeadSHA,
		createdAt,
	}, "\x00"))

	diffMetadata := mergeMetadata(params.Metadata, map[string]any{
		"source":   params.SourceType,
		"base_ref": params.BaseRef,
		"head_ref": params.HeadRef,
		"base_sha": params.BaseSHA,
		"head_sha": params.HeadSHA,
	})
	diffArtifact, err := s.artifacts.Save(ctx, artifact.SaveParams{
		ID:           stableID("artifact", snapshotID+"\x00diff"),
		WorkspaceID:  params.WorkspaceID,
		Kind:         "diff",
		RelativePath: filepath.ToSlash(filepath.Join("snapshots", snapshotID, "diff.patch")),
		ContentType:  "text/x-diff",
		MetadataJSON: mustJSON(diffMetadata),
		CreatedAt:    createdAt,
	}, params.Diff)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("save diff artifact: %w", err)
	}

	snapshotMetadata := mergeMetadata(params.Metadata, map[string]any{
		"file_count":  len(params.Files),
		"diff_sha256": diffArtifact.Sha256.String,
	})
	snapshot, err := s.queries.CreatePullRequestSnapshot(ctx, dbgen.CreatePullRequestSnapshotParams{
		ID:             snapshotID,
		RepositoryID:   params.RepositoryID,
		SourceType:     params.SourceType,
		Provider:       nullableString(params.Provider),
		Owner:          nullableString(params.Owner),
		Repo:           nullableString(params.Repo),
		PrNumber:       nullableInt64(params.PRNumber),
		PrTitle:        nullableString(params.PRTitle),
		PrUrl:          nullableString(params.PRURL),
		BaseRef:        nullableString(params.BaseRef),
		HeadRef:        nullableString(params.HeadRef),
		BaseSha:        nullableString(params.BaseSHA),
		HeadSha:        nullableString(params.HeadSHA),
		DiffArtifactID: nullableString(diffArtifact.ID),
		MetadataJson:   mustJSON(snapshotMetadata),
		CreatedAt:      createdAt,
	})
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("create pull request snapshot: %w", err)
	}

	result := SnapshotResult{
		Snapshot:     snapshot,
		DiffArtifact: diffArtifact,
	}
	for index, file := range params.Files {
		changedFile, patchArtifact, err := s.createChangedFile(ctx, params.WorkspaceID, snapshotID, params.SourceType, file, index, createdAt)
		if err != nil {
			return SnapshotResult{}, err
		}
		result.ChangedFiles = append(result.ChangedFiles, changedFile)
		if patchArtifact.ID != "" {
			result.PatchArtifacts = append(result.PatchArtifacts, patchArtifact)
		}
	}

	return result, nil
}

func (s *Service) createChangedFile(ctx context.Context, workspaceID string, snapshotID string, sourceType string, file ChangedFileInput, index int, createdAt string) (dbgen.ChangedFile, dbgen.Artifact, error) {
	lineRangesJSON := strings.TrimSpace(file.LineRangesJSON)
	if lineRangesJSON == "" {
		lineRangesJSON = "[]"
	}
	if !json.Valid([]byte(lineRangesJSON)) {
		return dbgen.ChangedFile{}, dbgen.Artifact{}, fmt.Errorf("changed file %s line ranges JSON is invalid", file.Path)
	}
	classification := fileclassify.Classify(fileclassify.Input{
		Path:          file.Path,
		OldPath:       file.OldPath,
		Binary:        file.IsBinary,
		ContentPrefix: []byte(file.Patch),
	})
	isBinary := file.IsBinary || classification.Binary
	isGenerated := file.IsGenerated || classification.Generated
	isExcluded := file.IsExcluded || classification.ExcludedCandidate

	var patchArtifact dbgen.Artifact
	if file.Patch != "" {
		artifactID := stableID("artifact", snapshotID+"\x00patch\x00"+file.Path)
		relativePath := filepath.ToSlash(filepath.Join(
			"snapshots",
			snapshotID,
			"patches",
			fmt.Sprintf("%03d-%s.patch", index+1, safeArtifactName(file.Path)),
		))
		metadata := mergeMetadata(file.Metadata, map[string]any{
			"source":   sourceType,
			"path":     file.Path,
			"old_path": file.OldPath,
			"status":   file.Status,
		})
		saved, err := s.artifacts.Save(ctx, artifact.SaveParams{
			ID:           artifactID,
			WorkspaceID:  workspaceID,
			Kind:         "patch",
			RelativePath: relativePath,
			ContentType:  "text/x-diff",
			MetadataJSON: mustJSON(metadata),
			CreatedAt:    createdAt,
		}, []byte(file.Patch))
		if err != nil {
			return dbgen.ChangedFile{}, dbgen.Artifact{}, fmt.Errorf("save patch artifact for %s: %w", file.Path, err)
		}
		patchArtifact = saved
	}

	changedFile, err := s.queries.CreateChangedFile(ctx, dbgen.CreateChangedFileParams{
		ID:              stableID("file", snapshotID+"\x00"+file.Path),
		SnapshotID:      snapshotID,
		Path:            file.Path,
		OldPath:         nullableString(file.OldPath),
		Status:          normalizeStatus(file.Status),
		Additions:       file.Additions,
		Deletions:       file.Deletions,
		IsBinary:        boolInt(isBinary),
		IsGenerated:     boolInt(isGenerated),
		IsExcluded:      boolInt(isExcluded),
		LineRangesJson:  lineRangesJSON,
		PatchArtifactID: nullableString(patchArtifact.ID),
		CreatedAt:       createdAt,
	})
	if err != nil {
		return dbgen.ChangedFile{}, dbgen.Artifact{}, fmt.Errorf("create changed file %s: %w", file.Path, err)
	}

	return changedFile, patchArtifact, nil
}

func (s *Service) validateSnapshotInput(ctx context.Context, params snapshotInput) error {
	if params.WorkspaceID == "" {
		return errors.New("workspace id is required")
	}
	if params.RepositoryID == "" {
		return errors.New("repository id is required")
	}
	if params.SourceType == "" {
		return errors.New("snapshot source type is required")
	}

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
		path := strings.TrimSpace(file.Path)
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

func nullableInt64(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func mergeMetadata(base map[string]any, values map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(values))
	for key, value := range base {
		if keepMetadataValue(value) {
			merged[key] = value
		}
	}
	for key, value := range values {
		if keepMetadataValue(value) {
			merged[key] = value
		}
	}
	return merged
}

func keepMetadataValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return typed != ""
	default:
		return true
	}
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
