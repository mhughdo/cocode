package diffparse

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Status string

const (
	StatusAdded    Status = "added"
	StatusModified Status = "modified"
	StatusDeleted  Status = "deleted"
	StatusRenamed  Status = "renamed"
	StatusBinary   Status = "binary"
)

type LineKind string

const (
	LineContext LineKind = "context"
	LineAdded   LineKind = "added"
	LineDeleted LineKind = "deleted"
)

type File struct {
	Path       string
	OldPath    string
	Status     Status
	Binary     bool
	Additions  int
	Deletions  int
	Hunks      []Hunk
	LineRanges []LineRange
}

type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Section  string
	Lines    []Line
}

type Line struct {
	Kind           LineKind
	OldLine        int
	NewLine        int
	Content        string
	NoNewlineAtEOF bool
}

type LineRange struct {
	Start int
	End   int
}

func (r LineRange) MarshalJSON() ([]byte, error) {
	if r.Start <= 0 || r.End <= 0 || r.End < r.Start {
		return nil, fmt.Errorf("invalid line range %d-%d", r.Start, r.End)
	}
	return json.Marshal([2]int{r.Start, r.End})
}

func Parse(input string) ([]File, error) {
	var files []File
	var current *File
	var hunk *Hunk
	var oldLine, newLine int

	lines := splitLines(input)
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "diff --git ") {
			current, hunk = startFile(&files, line), nil
			oldLine, newLine = 0, 0
			continue
		}
		if current == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			return nil, fmt.Errorf("line %d: expected diff header", i+1)
		}

		switch {
		case strings.HasPrefix(line, "@@ "):
			parsed, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			current.Hunks = append(current.Hunks, parsed)
			hunk = &current.Hunks[len(current.Hunks)-1]
			oldLine, newLine = hunk.OldStart, hunk.NewStart
		case strings.HasPrefix(line, "--- "):
			current.OldPath = normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))
			if current.OldPath == "" {
				current.Status = StatusAdded
			}
		case strings.HasPrefix(line, "+++ "):
			current.Path = normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
			if current.Path == "" {
				current.Status = StatusDeleted
			}
			current.pickDisplayPath()
		case strings.HasPrefix(line, "new file mode "):
			current.Status = StatusAdded
		case strings.HasPrefix(line, "deleted file mode "):
			current.Status = StatusDeleted
		case strings.HasPrefix(line, "rename from "):
			current.OldPath = strings.TrimPrefix(line, "rename from ")
			current.Status = StatusRenamed
			current.pickDisplayPath()
		case strings.HasPrefix(line, "rename to "):
			current.Path = strings.TrimPrefix(line, "rename to ")
			current.Status = StatusRenamed
			current.pickDisplayPath()
		case strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch"):
			current.Binary = true
			current.Status = StatusBinary
			hunk = nil
		case hunk != nil:
			if err := addHunkLine(current, hunk, line, &oldLine, &newLine); err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
		}
	}

	for i := range files {
		files[i].pickDisplayPath()
		if files[i].Status == "" {
			files[i].Status = StatusModified
		}
	}
	return files, nil
}

func (f File) LineRangesJSON() (string, error) {
	data, err := json.Marshal(f.LineRanges)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func startFile(files *[]File, line string) *File {
	oldPath, newPath := parseGitHeaderPaths(line)
	status := StatusModified
	if oldPath == "" {
		status = StatusAdded
	}
	if newPath == "" {
		status = StatusDeleted
	}

	*files = append(*files, File{
		Path:    newPath,
		OldPath: oldPath,
		Status:  status,
	})
	return &(*files)[len(*files)-1]
}

func (f *File) pickDisplayPath() {
	if f.Status == "" {
		f.Status = StatusModified
	}
	if f.Status == StatusDeleted && f.Path == "" {
		f.Path = f.OldPath
	}
	if f.Status == StatusDeleted && f.OldPath == f.Path {
		f.OldPath = ""
	}
	if f.Status != StatusRenamed && f.OldPath == f.Path {
		f.OldPath = ""
	}
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(?: ?(.*))?$`)

func parseHunkHeader(line string) (Hunk, error) {
	matches := hunkHeaderPattern.FindStringSubmatch(line)
	if matches == nil {
		return Hunk{}, fmt.Errorf("invalid hunk header %q", line)
	}

	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return Hunk{}, err
	}
	oldLines, err := parseCount(matches[2])
	if err != nil {
		return Hunk{}, err
	}
	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return Hunk{}, err
	}
	newLines, err := parseCount(matches[4])
	if err != nil {
		return Hunk{}, err
	}

	return Hunk{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
		Section:  matches[5],
	}, nil
}

func parseCount(value string) (int, error) {
	if value == "" {
		return 1, nil
	}
	return strconv.Atoi(value)
}

func addHunkLine(file *File, hunk *Hunk, line string, oldLine *int, newLine *int) error {
	if line == `\ No newline at end of file` {
		if len(hunk.Lines) == 0 {
			return errors.New("no-newline marker without preceding line")
		}
		hunk.Lines[len(hunk.Lines)-1].NoNewlineAtEOF = true
		return nil
	}
	if line == "" {
		return errors.New("empty line in hunk")
	}

	switch line[0] {
	case ' ':
		hunk.Lines = append(hunk.Lines, Line{
			Kind:    LineContext,
			OldLine: *oldLine,
			NewLine: *newLine,
			Content: line[1:],
		})
		(*oldLine)++
		(*newLine)++
	case '+':
		hunk.Lines = append(hunk.Lines, Line{
			Kind:    LineAdded,
			NewLine: *newLine,
			Content: line[1:],
		})
		file.Additions++
		file.addLineRange(*newLine)
		(*newLine)++
	case '-':
		hunk.Lines = append(hunk.Lines, Line{
			Kind:    LineDeleted,
			OldLine: *oldLine,
			Content: line[1:],
		})
		file.Deletions++
		(*oldLine)++
	default:
		return fmt.Errorf("unexpected hunk line %q", line)
	}
	return nil
}

func (f *File) addLineRange(line int) {
	if line <= 0 {
		return
	}
	last := len(f.LineRanges) - 1
	if last >= 0 && f.LineRanges[last].End+1 == line {
		f.LineRanges[last].End = line
		return
	}
	f.LineRanges = append(f.LineRanges, LineRange{Start: line, End: line})
}

func parseGitHeaderPaths(line string) (string, string) {
	rest := strings.TrimPrefix(line, "diff --git ")
	first, remaining := takePathSpec(rest)
	second, _ := takePathSpec(strings.TrimSpace(remaining))
	return normalizeDiffPath(first), normalizeDiffPath(second)
}

func takePathSpec(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if strings.HasPrefix(value, `"`) {
		unquoted, rest, ok := takeQuoted(value)
		if ok {
			return unquoted, rest
		}
	}
	if strings.HasPrefix(value, "a/") {
		if index := strings.Index(value[2:], " b/"); index >= 0 {
			split := 2 + index
			return value[:split], value[split+1:]
		}
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.TrimSpace(strings.TrimPrefix(value, fields[0]))
}

func takeQuoted(value string) (string, string, bool) {
	for i := 1; i < len(value); i++ {
		if value[i] != '"' {
			continue
		}
		candidate := value[:i+1]
		unquoted, err := strconv.Unquote(candidate)
		if err == nil {
			return unquoted, value[i+1:], true
		}
	}
	return "", "", false
}

func normalizeDiffPath(path string) string {
	path = trimPathTimestamp(path)
	if path == "" || path == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	if strings.HasPrefix(path, `"`) {
		if unquoted, _, ok := takeQuoted(path); ok {
			return normalizeDiffPath(unquoted)
		}
	}
	return path
}

func trimPathTimestamp(path string) string {
	if index := strings.IndexByte(path, '\t'); index >= 0 {
		return path[:index]
	}
	return path
}

func splitLines(input string) []string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	if strings.HasSuffix(input, "\n") {
		input = strings.TrimSuffix(input, "\n")
	}
	if input == "" {
		return nil
	}
	return strings.Split(input, "\n")
}
