package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrPathEscapesRoot = errors.New("path escapes root")

func ResolveRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve root symlinks: %w", err)
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect root: %w", err)
	}
	if !stat.IsDir() {
		return "", errors.New("root must be a directory")
	}
	return resolved, nil
}

func SafePathSegment(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return false
	}
	return value == filepath.Base(value) && !strings.ContainsAny(value, `/\`)
}

func CleanRelativePath(path string) (string, error) {
	path = strings.TrimSpace(filepath.ToSlash(strings.ReplaceAll(path, "\\", "/")))
	path = strings.TrimPrefix(path, "./")
	if path == "" || path == "." {
		return ".", nil
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, path)
		}
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, path)
	}
	return clean, nil
}

func JoinWithinRoot(root string, relativePath string) (string, string, error) {
	resolvedRoot, err := ResolveRoot(root)
	if err != nil {
		return "", "", err
	}
	clean, err := CleanRelativePath(relativePath)
	if err != nil {
		return "", "", err
	}
	if clean == "." {
		return "", "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, relativePath)
	}
	target := filepath.Join(resolvedRoot, filepath.FromSlash(clean))
	if !PathInsideRoot(resolvedRoot, target) {
		return "", "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, relativePath)
	}
	return target, clean, nil
}

func ResolveExistingWithinRoot(root string, relativePath string) (string, string, error) {
	target, clean, err := JoinWithinRoot(root, relativePath)
	if err != nil {
		return "", "", err
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", fmt.Errorf("resolve path symlinks: %w", err)
	}
	resolvedRoot, err := ResolveRoot(root)
	if err != nil {
		return "", "", err
	}
	if !PathInsideRoot(resolvedRoot, resolved) {
		return "", "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, relativePath)
	}
	return resolved, clean, nil
}

func ResolveWriteWithinRoot(root string, relativePath string) (string, string, error) {
	target, clean, err := JoinWithinRoot(root, relativePath)
	if err != nil {
		return "", "", err
	}
	parent := filepath.Dir(target)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", "", fmt.Errorf("resolve parent symlinks: %w", err)
	}
	resolvedRoot, err := ResolveRoot(root)
	if err != nil {
		return "", "", err
	}
	if !PathInsideRoot(resolvedRoot, resolvedParent) {
		return "", "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, relativePath)
	}
	return filepath.Join(resolvedParent, filepath.Base(target)), clean, nil
}

func PathInsideRoot(root string, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
