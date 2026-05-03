package contextbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/diffparse"
)

type DiffContextFile struct {
	ChangedFileID   string
	PatchArtifactID string
	Path            string
	OldPath         string
	Status          string
	Binary          bool
	Excluded        bool
	Hunks           []diffparse.Hunk
}

func DiffContextFileFromParsed(changedFileID string, patchArtifactID string, file diffparse.File) DiffContextFile {
	return DiffContextFile{
		ChangedFileID:   changedFileID,
		PatchArtifactID: patchArtifactID,
		Path:            file.Path,
		OldPath:         file.OldPath,
		Status:          string(file.Status),
		Binary:          file.Binary,
		Hunks:           append([]diffparse.Hunk(nil), file.Hunks...),
	}
}

func BuildDiffContextItems(bundleID string, files []DiffContextFile) ([]Item, error) {
	if strings.TrimSpace(bundleID) == "" {
		return nil, fmt.Errorf("context bundle id is required")
	}
	items := []Item{}
	for _, file := range files {
		if file.Binary || file.Excluded || len(file.Hunks) == 0 {
			continue
		}
		path := strings.TrimSpace(file.Path)
		if path == "" {
			path = strings.TrimSpace(file.OldPath)
		}
		if path == "" {
			return nil, fmt.Errorf("diff context file path is required")
		}
		for index, hunk := range file.Hunks {
			content := RenderDiffHunk(hunk)
			metadata, err := diffHunkMetadata(file, hunk, index)
			if err != nil {
				return nil, err
			}
			startLine, endLine := hunkDisplayRange(hunk)
			title := fmt.Sprintf("%s hunk %d", path, index+1)
			if strings.TrimSpace(hunk.Section) != "" {
				title = fmt.Sprintf("%s: %s", title, strings.TrimSpace(hunk.Section))
			}
			item := Item{
				ID:              stableContextItemID(bundleID, path, index),
				ContextBundleID: bundleID,
				Kind:            ItemChangedHunk,
				Path:            path,
				StartLine:       startLine,
				EndLine:         endLine,
				Title:           title,
				Content:         content,
				TokenEstimate:   EstimateContentTokens(content),
				Metadata:        metadata,
			}
			if err := item.Validate(); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func RenderDiffHunk(hunk diffparse.Hunk) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines))
	if strings.TrimSpace(hunk.Section) != "" {
		out.WriteByte(' ')
		out.WriteString(strings.TrimSpace(hunk.Section))
	}
	out.WriteByte('\n')
	for _, line := range hunk.Lines {
		switch line.Kind {
		case diffparse.LineAdded:
			out.WriteByte('+')
		case diffparse.LineDeleted:
			out.WriteByte('-')
		default:
			out.WriteByte(' ')
		}
		out.WriteString(line.Content)
		out.WriteByte('\n')
		if line.NoNewlineAtEOF {
			out.WriteString("\\ No newline at end of file\n")
		}
	}
	return out.String()
}

func diffHunkMetadata(file DiffContextFile, hunk diffparse.Hunk, index int) (json.RawMessage, error) {
	payload := map[string]any{
		"changed_file_id":   file.ChangedFileID,
		"patch_artifact_id": file.PatchArtifactID,
		"old_path":          file.OldPath,
		"status":            file.Status,
		"hunk_index":        index,
		"old_start":         hunk.OldStart,
		"old_lines":         hunk.OldLines,
		"new_start":         hunk.NewStart,
		"new_lines":         hunk.NewLines,
	}
	if strings.TrimSpace(hunk.Section) != "" {
		payload["section"] = strings.TrimSpace(hunk.Section)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode diff hunk metadata: %w", err)
	}
	return data, nil
}

func hunkDisplayRange(hunk diffparse.Hunk) (int64, int64) {
	if hunk.NewLines > 0 {
		return int64(hunk.NewStart), int64(hunk.NewStart + hunk.NewLines - 1)
	}
	if hunk.OldLines > 0 {
		return int64(hunk.OldStart), int64(hunk.OldStart + hunk.OldLines - 1)
	}
	return 0, 0
}

func stableContextItemID(bundleID string, path string, hunkIndex int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", bundleID, path, hunkIndex)))
	return "context_item_" + hex.EncodeToString(sum[:8])
}
