package gitrepo

import (
	"context"
	"sort"
	"strings"
)

type Branch struct {
	Name      string
	CommitSHA string
	Upstream  string
	Current   bool
	Remote    bool
}

func (c Collector) ListBranches(ctx context.Context, selectedPath string) ([]Branch, error) {
	runner := c.runner()
	info, err := validate(ctx, runner, selectedPath)
	if err != nil {
		return nil, err
	}
	result, err := runner.RunRaw(ctx, info.RootPath, "for-each-ref", "--format=%(refname)%09%(refname:short)%09%(objectname)%09%(upstream:short)%09%(HEAD)", "refs/heads/", "refs/remotes/")
	if err != nil {
		return nil, err
	}
	return parseBranchRefs(result.Stdout), nil
}

func parseBranchRefs(output string) []Branch {
	seen := map[string]Branch{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		headMarker := ""
		if len(parts) > 4 {
			headMarker = strings.TrimSpace(parts[4])
		}
		fullRef := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if name == "" || strings.HasSuffix(name, "/HEAD") {
			continue
		}
		branch := Branch{
			Name:      name,
			CommitSHA: strings.TrimSpace(parts[2]),
			Upstream:  strings.TrimSpace(parts[3]),
			Current:   headMarker == "*",
			Remote:    strings.HasPrefix(fullRef, "refs/remotes/"),
		}
		existing, ok := seen[name]
		if ok && branch.Remote && !existing.Remote {
			continue
		}
		if ok && !branch.Current && existing.Current {
			continue
		}
		seen[name] = branch
	}

	branches := make([]Branch, 0, len(seen))
	for _, branch := range seen {
		branches = append(branches, branch)
	}
	sort.Slice(branches, func(i, j int) bool {
		left := branches[i]
		right := branches[j]
		if left.Current != right.Current {
			return left.Current
		}
		if left.Remote != right.Remote {
			return !left.Remote
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
	return branches
}
