package githubpr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

type GHRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecGHRunner struct{}

func (ExecGHRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return nil, apperror.InvalidRequest("GitHub CLI is not available. Install gh and run gh auth login.")
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	return nil, apperror.InvalidRequest("GitHub CLI command failed: " + firstLine(message))
}

type GHClient struct {
	Command string
	Runner  GHRunner
}

func (c GHClient) FetchMetadata(ctx context.Context, ref Reference) (Metadata, error) {
	if err := validateReference(ref); err != nil {
		return Metadata{}, err
	}
	body, err := c.run(ctx, "pr", "view", strconv.FormatInt(ref.Number, 10), "--repo", ghRepo(ref), "--json", "title,url,author,baseRefName,headRefName,baseRefOid,headRefOid")
	if err != nil {
		body, err = c.run(ctx, "pr", "view", strconv.FormatInt(ref.Number, 10), "--repo", ghRepo(ref), "--json", "title,url,author,baseRefName,headRefName")
	}
	if err != nil {
		return Metadata{}, err
	}
	var view struct {
		Title  string `json:"title"`
		URL    string `json:"url"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		BaseRefName string `json:"baseRefName"`
		HeadRefName string `json:"headRefName"`
		BaseRefOID  string `json:"baseRefOid"`
		HeadRefOID  string `json:"headRefOid"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		return Metadata{}, apperror.Internal("failed to decode GitHub CLI pull request metadata")
	}
	if strings.TrimSpace(view.URL) == "" {
		view.URL = ref.CanonicalURL
	}
	return Metadata{
		Owner:   ref.Owner,
		Repo:    ref.Repo,
		Number:  ref.Number,
		Title:   view.Title,
		Author:  view.Author.Login,
		URL:     view.URL,
		BaseRef: view.BaseRefName,
		HeadRef: view.HeadRefName,
		BaseSHA: view.BaseRefOID,
		HeadSHA: view.HeadRefOID,
	}, nil
}

func (c GHClient) FetchChangedFiles(ctx context.Context, ref Reference) ([]ChangedFile, error) {
	diff, err := c.FetchDiff(ctx, ref)
	if err != nil {
		return nil, err
	}
	files := ChangedFilesFromUnifiedDiff(diff)
	if len(files) > 0 {
		return files, nil
	}
	body, err := c.run(ctx, "pr", "view", strconv.FormatInt(ref.Number, 10), "--repo", ghRepo(ref), "--json", "files")
	if err != nil {
		return nil, err
	}
	return changedFilesFromGHView(body)
}

func (c GHClient) FetchDiff(ctx context.Context, ref Reference) ([]byte, error) {
	if err := validateReference(ref); err != nil {
		return nil, err
	}
	return c.run(ctx, "pr", "diff", strconv.FormatInt(ref.Number, 10), "--repo", ghRepo(ref), "--patch")
}

func (c GHClient) FetchPreviousComments(ctx context.Context, ref Reference) (PreviousComments, error) {
	if err := validateReference(ref); err != nil {
		return PreviousComments{}, err
	}
	issueComments, err := ghAPIList[issueComment](ctx, c, fmt.Sprintf("repos/%s/%s/issues/%d/comments", ref.Owner, ref.Repo, ref.Number))
	if err != nil {
		return PreviousComments{}, err
	}
	reviewComments, err := ghAPIList[reviewComment](ctx, c, fmt.Sprintf("repos/%s/%s/pulls/%d/comments", ref.Owner, ref.Repo, ref.Number))
	if err != nil {
		return PreviousComments{}, err
	}
	reviews, err := ghAPIList[pullReview](ctx, c, fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", ref.Owner, ref.Repo, ref.Number))
	if err != nil {
		return PreviousComments{}, err
	}
	return previousCommentsFromGitHub(issueComments, reviewComments, reviews), nil
}

func ghAPIList[T any](ctx context.Context, c GHClient, path string) ([]T, error) {
	body, err := c.run(ctx, "api", "--paginate", "--slurp", path)
	if err != nil {
		return nil, err
	}
	items, err := decodeGHAPIList[T](body)
	if err != nil {
		return nil, apperror.Internal("failed to decode GitHub CLI API response")
	}
	return items, nil
}

func (c GHClient) run(ctx context.Context, args ...string) ([]byte, error) {
	runner := c.Runner
	if runner == nil {
		runner = ExecGHRunner{}
	}
	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = "gh"
	}
	return runner.Run(ctx, command, args...)
}

func validateReference(ref Reference) error {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return apperror.InvalidRequest("GitHub pull request reference is invalid")
	}
	return nil
}

func ghRepo(ref Reference) string {
	return ref.Owner + "/" + ref.Repo
}

func firstLine(value string) string {
	line := strings.TrimSpace(value)
	if index := strings.IndexByte(line, '\n'); index >= 0 {
		line = strings.TrimSpace(line[:index])
	}
	if line == "" {
		return "unknown error"
	}
	return line
}

func decodeGHAPIList[T any](content []byte) ([]T, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return []T{}, nil
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(trimmed, &rawItems); err != nil {
		return nil, err
	}
	items := make([]T, 0, len(rawItems))
	for _, raw := range rawItems {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			continue
		}
		if raw[0] == '[' {
			var page []T
			if err := json.Unmarshal(raw, &page); err != nil {
				return nil, err
			}
			items = append(items, page...)
			continue
		}
		var item T
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func changedFilesFromGHView(content []byte) ([]ChangedFile, error) {
	var body struct {
		Files json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(content, &body); err != nil {
		return nil, apperror.Internal("failed to decode GitHub CLI pull request files")
	}
	type ghFileView struct {
		Path      string `json:"path"`
		Additions int64  `json:"additions"`
		Deletions int64  `json:"deletions"`
	}
	var files []ghFileView
	if err := json.Unmarshal(body.Files, &files); err != nil {
		var nested struct {
			Nodes []ghFileView `json:"nodes"`
		}
		if nestedErr := json.Unmarshal(body.Files, &nested); nestedErr != nil {
			return nil, apperror.Internal("failed to decode GitHub CLI pull request files")
		}
		files = nested.Nodes
	}
	result := make([]ChangedFile, 0, len(files))
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		result = append(result, ChangedFile{
			Filename:  path,
			Status:    "modified",
			Additions: file.Additions,
			Deletions: file.Deletions,
			Changes:   file.Additions + file.Deletions,
		})
	}
	return result, nil
}

type diffFileChunk struct {
	headerPath       string
	oldPath          string
	newPath          string
	previousFilename string
	status           string
	additions        int64
	deletions        int64
	lines            []string
}

func ChangedFilesFromUnifiedDiff(diff []byte) []ChangedFile {
	lines := strings.SplitAfter(string(diff), "\n")
	files := make([]ChangedFile, 0)
	var current *diffFileChunk

	flush := func() {
		if current == nil || len(current.lines) == 0 {
			return
		}
		path := current.newPath
		if path == "" {
			path = current.oldPath
		}
		if path == "" {
			path = current.headerPath
		}
		path = strings.TrimSpace(path)
		if path == "" {
			current = nil
			return
		}
		status := current.status
		switch {
		case status == "":
			status = "modified"
		case status == "deleted":
			status = "removed"
		}
		files = append(files, ChangedFile{
			Filename:         path,
			PreviousFilename: current.previousFilename,
			Status:           status,
			Additions:        current.additions,
			Deletions:        current.deletions,
			Changes:          current.additions + current.deletions,
			Patch:            strings.Join(current.lines, ""),
		})
		current = nil
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			current = &diffFileChunk{
				headerPath: diffHeaderPath(line),
				status:     "modified",
			}
		}
		if current == nil {
			continue
		}
		current.lines = append(current.lines, line)
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "new file mode "):
			current.status = "added"
		case strings.HasPrefix(trimmed, "deleted file mode "):
			current.status = "removed"
		case strings.HasPrefix(trimmed, "rename from "):
			current.status = "renamed"
			current.previousFilename = normalizeDiffPath(strings.TrimPrefix(trimmed, "rename from "))
		case strings.HasPrefix(trimmed, "rename to "):
			current.status = "renamed"
			current.newPath = normalizeDiffPath(strings.TrimPrefix(trimmed, "rename to "))
		case strings.HasPrefix(trimmed, "--- "):
			path := normalizeDiffPath(strings.TrimPrefix(trimmed, "--- "))
			if path == "" {
				current.status = "added"
			}
			current.oldPath = path
		case strings.HasPrefix(trimmed, "+++ "):
			path := normalizeDiffPath(strings.TrimPrefix(trimmed, "+++ "))
			if path == "" {
				current.status = "removed"
			}
			current.newPath = path
		case strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++"):
			current.additions++
		case strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---"):
			current.deletions++
		}
	}
	flush()
	return files
}

func diffHeaderPath(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) >= 4 {
		return normalizeDiffPath(fields[3])
	}
	return ""
}

func normalizeDiffPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "/dev/null" {
		return ""
	}
	if unquoted, err := strconv.Unquote(path); err == nil {
		path = unquoted
	}
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return strings.TrimSpace(path)
}
