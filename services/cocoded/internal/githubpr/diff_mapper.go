package githubpr

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

const (
	SideRight   = "RIGHT"
	SideLeft    = "LEFT"
	SideUnknown = "UNKNOWN"
)

var (
	ErrInvalidDiffAnchor = errors.New("diff anchor request is invalid")
	ErrDiffAnchorMissing = errors.New("diff anchor was not found")
)

type DiffAnchorRequest struct {
	Path string
	Line int
	Side string
}

type DiffAnchor struct {
	Path     string
	Line     int
	Side     string
	Position int
}

func MapDiffLine(files []diffparse.File, request DiffAnchorRequest) (DiffAnchor, error) {
	request.Path = normalizeAnchorPath(request.Path)
	if request.Path == "" {
		return DiffAnchor{}, fmt.Errorf("%w: path is required", ErrInvalidDiffAnchor)
	}
	if request.Line <= 0 {
		return DiffAnchor{}, fmt.Errorf("%w: line must be positive", ErrInvalidDiffAnchor)
	}
	side, err := normalizeAnchorSide(request.Side)
	if err != nil {
		return DiffAnchor{}, err
	}
	if side == SideUnknown {
		if anchor, err := mapDiffLineOnSide(files, request, SideRight); err == nil {
			return anchor, nil
		}
		return mapDiffLineOnSide(files, request, SideLeft)
	}
	return mapDiffLineOnSide(files, request, side)
}

func MapDiffLineFromPatch(patch string, request DiffAnchorRequest) (DiffAnchor, error) {
	files, err := diffparse.Parse(patch)
	if err != nil {
		return DiffAnchor{}, fmt.Errorf("parse diff: %w", err)
	}
	return MapDiffLine(files, request)
}

func mapDiffLineOnSide(files []diffparse.File, request DiffAnchorRequest, side string) (DiffAnchor, error) {
	for _, file := range files {
		if !fileMatchesAnchor(file, request.Path, side) {
			continue
		}
		if anchor, ok := mapFileLine(file, request.Line, side); ok {
			return anchor, nil
		}
		return DiffAnchor{}, fmt.Errorf("%w: line %d is not in the patch for %s", ErrDiffAnchorMissing, request.Line, request.Path)
	}
	return DiffAnchor{}, fmt.Errorf("%w: file %s is not in the patch", ErrDiffAnchorMissing, request.Path)
}

func mapFileLine(file diffparse.File, lineNumber int, side string) (DiffAnchor, bool) {
	position := 0
	for hunkIndex, hunk := range file.Hunks {
		if hunkIndex > 0 {
			position++
		}
		for _, line := range hunk.Lines {
			position++
			if !lineMatchesSide(line, lineNumber, side) {
				continue
			}
			return DiffAnchor{
				Path:     githubPathForFile(file),
				Line:     lineNumber,
				Side:     side,
				Position: position,
			}, true
		}
	}
	return DiffAnchor{}, false
}

func lineMatchesSide(line diffparse.Line, lineNumber int, side string) bool {
	switch side {
	case SideRight:
		return line.Kind != diffparse.LineDeleted && line.NewLine == lineNumber
	case SideLeft:
		return line.Kind != diffparse.LineAdded && line.OldLine == lineNumber
	default:
		return false
	}
}

func fileMatchesAnchor(file diffparse.File, path string, side string) bool {
	path = normalizeAnchorPath(path)
	candidates := []string{file.Path}
	if side == SideLeft || file.Status == diffparse.StatusRenamed || file.Status == diffparse.StatusDeleted {
		candidates = append(candidates, file.OldPath)
	}
	for _, candidate := range candidates {
		if normalizeAnchorPath(candidate) == path {
			return true
		}
	}
	return false
}

func githubPathForFile(file diffparse.File) string {
	if strings.TrimSpace(file.Path) != "" {
		return file.Path
	}
	return file.OldPath
}

func normalizeAnchorSide(side string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "", SideUnknown:
		return SideUnknown, nil
	case SideRight:
		return SideRight, nil
	case SideLeft:
		return SideLeft, nil
	default:
		return "", fmt.Errorf("%w: unsupported side %q", ErrInvalidDiffAnchor, side)
	}
}

func normalizeAnchorPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}
