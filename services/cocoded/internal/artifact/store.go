package artifact

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/security"
)

const defaultContentType = "text/plain"

type Store struct {
	root    string
	queries *dbgen.Queries
}

type SaveParams struct {
	ID              string
	WorkspaceID     string
	ReviewSessionID sql.NullString
	Kind            string
	RelativePath    string
	ContentType     string
	MetadataJSON    string
	CreatedAt       string
}

func New(root string, queries *dbgen.Queries) (*Store, error) {
	if root == "" {
		return nil, errors.New("artifact root is required")
	}
	if queries == nil {
		return nil, errors.New("artifact queries are required")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact root: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact root: %w", err)
	}
	resolvedRoot, err := security.ResolveRoot(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact root symlinks: %w", err)
	}
	return &Store{root: resolvedRoot, queries: queries}, nil
}

func (s *Store) Save(ctx context.Context, params SaveParams, content []byte) (dbgen.Artifact, error) {
	target, err := s.pathFor(params.WorkspaceID, params.RelativePath)
	if err != nil {
		return dbgen.Artifact{}, err
	}
	if params.ID == "" {
		return dbgen.Artifact{}, errors.New("artifact id is required")
	}
	if params.Kind == "" {
		return dbgen.Artifact{}, errors.New("artifact kind is required")
	}
	if params.ContentType == "" {
		params.ContentType = defaultContentType
	}
	if params.MetadataJSON == "" {
		params.MetadataJSON = "{}"
	}
	if params.CreatedAt == "" {
		params.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return dbgen.Artifact{}, fmt.Errorf("create artifact directory: %w", err)
	}
	target, err = s.writePathFor(params.WorkspaceID, params.RelativePath)
	if err != nil {
		return dbgen.Artifact{}, err
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return dbgen.Artifact{}, fmt.Errorf("write artifact file: %w", err)
	}
	cleanRelative, err := artifactRelativePath(params.RelativePath)
	if err != nil {
		_ = os.Remove(target)
		return dbgen.Artifact{}, err
	}

	digest := sha256.Sum256(content)
	artifact, err := s.queries.CreateArtifact(ctx, dbgen.CreateArtifactParams{
		ID:              params.ID,
		WorkspaceID:     params.WorkspaceID,
		ReviewSessionID: params.ReviewSessionID,
		Kind:            params.Kind,
		RelativePath:    cleanRelative,
		ContentType:     params.ContentType,
		SizeBytes:       int64(len(content)),
		Sha256:          sql.NullString{String: hex.EncodeToString(digest[:]), Valid: true},
		MetadataJson:    params.MetadataJSON,
		CreatedAt:       params.CreatedAt,
	})
	if err != nil {
		_ = os.Remove(target)
		return dbgen.Artifact{}, fmt.Errorf("create artifact metadata: %w", err)
	}

	return artifact, nil
}

func (s *Store) Read(ctx context.Context, id string) ([]byte, dbgen.Artifact, error) {
	artifact, err := s.queries.GetArtifact(ctx, id)
	if err != nil {
		return nil, dbgen.Artifact{}, fmt.Errorf("get artifact metadata: %w", err)
	}
	target, err := s.readPathFor(artifact.WorkspaceID, artifact.RelativePath)
	if err != nil {
		return nil, dbgen.Artifact{}, err
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return nil, dbgen.Artifact{}, fmt.Errorf("read artifact file: %w", err)
	}
	return content, artifact, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	artifact, err := s.queries.GetArtifact(ctx, id)
	if err != nil {
		return fmt.Errorf("get artifact metadata: %w", err)
	}
	target, err := s.pathFor(artifact.WorkspaceID, artifact.RelativePath)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove artifact file: %w", err)
	}
	if err := s.queries.DeleteArtifact(ctx, id); err != nil {
		return fmt.Errorf("delete artifact metadata: %w", err)
	}
	return nil
}

func (s *Store) pathFor(workspaceID string, relativePath string) (string, error) {
	if s == nil {
		return "", errors.New("artifact store is required")
	}
	if workspaceID == "" {
		return "", errors.New("workspace id is required")
	}
	if relativePath == "" {
		return "", errors.New("artifact relative path is required")
	}
	if !security.SafePathSegment(workspaceID) {
		return "", fmt.Errorf("unsafe workspace id %q", workspaceID)
	}
	cleanRelative, err := artifactRelativePath(relativePath)
	if err != nil {
		return "", fmt.Errorf("unsafe artifact relative path %q", relativePath)
	}
	target, _, err := security.JoinWithinRoot(s.root, filepath.ToSlash(filepath.Join(workspaceID, cleanRelative)))
	if err != nil {
		return "", fmt.Errorf("validate artifact path: %w", err)
	}
	return target, nil
}

func artifactRelativePath(relativePath string) (string, error) {
	cleanRelative, err := security.CleanRelativePath(relativePath)
	if err != nil || cleanRelative == "." {
		return "", fmt.Errorf("unsafe artifact relative path %q", relativePath)
	}
	return cleanRelative, nil
}

func (s *Store) readPathFor(workspaceID string, relativePath string) (string, error) {
	if !security.SafePathSegment(workspaceID) {
		return "", fmt.Errorf("unsafe workspace id %q", workspaceID)
	}
	target, _, err := security.ResolveExistingWithinRoot(s.root, filepath.ToSlash(filepath.Join(workspaceID, relativePath)))
	if err != nil {
		return "", fmt.Errorf("validate artifact path: %w", err)
	}
	return target, nil
}

func (s *Store) writePathFor(workspaceID string, relativePath string) (string, error) {
	if !security.SafePathSegment(workspaceID) {
		return "", fmt.Errorf("unsafe workspace id %q", workspaceID)
	}
	target, _, err := security.ResolveWriteWithinRoot(s.root, filepath.ToSlash(filepath.Join(workspaceID, relativePath)))
	if err != nil {
		return "", fmt.Errorf("validate artifact path: %w", err)
	}
	return target, nil
}
