package findingengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hughdo/cocode/services/cocoded/internal/agentoutput"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

type lineRange struct {
	start int64
	end   int64
}

func NormalizeCandidate(candidate agentoutput.Candidate, changedFiles []dbgen.ChangedFile) agentoutput.Candidate {
	files := changedFileIndex(changedFiles)
	for index := range candidate.Locations {
		location := &candidate.Locations[index]
		normalizedPath, pathOK := normalizePath(location.Path)
		location.Path = normalizedPath
		if !pathOK {
			valid := false
			location.Valid = &valid
			location.Message = "path is unsafe or empty"
			continue
		}
		file, ok := matchChangedFile(*location, files)
		valid := ok && lineRangeValid(*location, file)
		location.Valid = &valid
		if ok {
			location.ChangedFileID = file.file.ID
			if !valid {
				location.Message = "line range is outside changed ranges"
			}
		} else {
			location.Message = "path is not part of the reviewed diff"
		}
	}
	if len(candidate.Locations) > 0 {
		primary := candidate.Locations[0]
		for _, location := range candidate.Locations {
			if location.Valid != nil && *location.Valid {
				primary = location
				break
			}
		}
		candidate.PrimaryPath = primary.Path
		candidate.PrimaryStartLine = primary.StartLine
		candidate.PrimaryEndLine = primary.EndLine
	}
	candidate.Fingerprint = Fingerprint(candidate)
	return candidate
}

func Fingerprint(candidate agentoutput.Candidate) string {
	path, ok := normalizePath(candidate.PrimaryPath)
	if !ok {
		path = "unlocated"
	}
	parts := []string{
		"finding:v1",
		strings.ToLower(strings.TrimSpace(candidate.Category)),
		path,
		lineBucket(candidate.PrimaryStartLine),
		strings.Join(claimTerms(candidate.Claim), " "),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "fp_" + hex.EncodeToString(digest[:8])
}

type indexedChangedFile struct {
	file   dbgen.ChangedFile
	ranges []lineRange
	left   bool
}

func changedFileIndex(changedFiles []dbgen.ChangedFile) map[string]indexedChangedFile {
	files := map[string]indexedChangedFile{}
	basenames := map[string][]indexedChangedFile{}
	for _, changedFile := range changedFiles {
		indexed := indexedChangedFile{
			file:   changedFile,
			ranges: parseLineRanges(changedFile.LineRangesJson),
		}
		if path, ok := normalizePath(changedFile.Path); ok {
			files[path] = indexed
			basenames[filepath.Base(path)] = append(basenames[filepath.Base(path)], indexed)
		}
		if changedFile.OldPath.Valid {
			if oldPath, ok := normalizePath(changedFile.OldPath.String); ok {
				oldIndexed := indexed
				oldIndexed.left = true
				files[oldPath] = oldIndexed
				basenames[filepath.Base(oldPath)] = append(basenames[filepath.Base(oldPath)], oldIndexed)
			}
		}
	}
	for basename, matches := range basenames {
		if len(matches) == 1 {
			files[basename] = matches[0]
		}
	}
	return files
}

func matchChangedFile(location agentoutput.CandidateLocation, files map[string]indexedChangedFile) (indexedChangedFile, bool) {
	path, ok := normalizePath(location.Path)
	if !ok {
		return indexedChangedFile{}, false
	}
	file, ok := files[path]
	return file, ok
}

func lineRangeValid(location agentoutput.CandidateLocation, file indexedChangedFile) bool {
	if location.StartLine < 1 || location.EndLine < location.StartLine {
		return false
	}
	if file.left || strings.EqualFold(location.Side, "LEFT") {
		return true
	}
	if len(file.ranges) == 0 {
		return true
	}
	for _, changed := range file.ranges {
		if location.StartLine <= changed.end && location.EndLine >= changed.start {
			return true
		}
	}
	return false
}

func parseLineRanges(raw string) []lineRange {
	var encoded [][]int64
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil {
		return nil
	}
	ranges := make([]lineRange, 0, len(encoded))
	for _, item := range encoded {
		if len(item) != 2 || item[0] < 1 || item[1] < item[0] {
			continue
		}
		ranges = append(ranges, lineRange{start: item[0], end: item[1]})
	}
	return ranges
}

func normalizePath(value string) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "a/")
	value = strings.TrimPrefix(value, "b/")
	if value == "" || strings.HasPrefix(value, "/") {
		return "", false
	}
	cleaned := filepath.ToSlash(filepath.Clean(value))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func lineBucket(line int64) string {
	if line < 1 {
		return "unknown"
	}
	return strconv.FormatInt((line/10)*10, 10)
}

func Overlap(aStart int64, aEnd int64, bStart int64, bEnd int64) bool {
	return aStart <= bEnd && bStart <= aEnd
}

func SimilarClaims(a string, b string) bool {
	aTerms := claimTerms(a)
	bTerms := claimTerms(b)
	if len(aTerms) == 0 || len(bTerms) == 0 {
		return false
	}
	aSet := map[string]bool{}
	for _, term := range aTerms {
		aSet[term] = true
	}
	intersection := 0
	for _, term := range bTerms {
		if aSet[term] {
			intersection++
		}
	}
	minLen := len(aTerms)
	if len(bTerms) < minLen {
		minLen = len(bTerms)
	}
	return float64(intersection)/float64(minLen) >= 0.5
}

func claimTerms(claim string) []string {
	seen := map[string]bool{}
	terms := []string{}
	for _, token := range strings.FieldsFunc(strings.ToLower(claim), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(token) < 3 || claimStopWords[token] || seen[token] {
			continue
		}
		seen[token] = true
		terms = append(terms, token)
	}
	sort.Strings(terms)
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

var claimStopWords = map[string]bool{
	"the":     true,
	"and":     true,
	"for":     true,
	"with":    true,
	"without": true,
	"can":     true,
	"could":   true,
	"should":  true,
	"this":    true,
	"that":    true,
	"into":    true,
	"from":    true,
	"before":  true,
	"after":   true,
	"when":    true,
	"while":   true,
	"are":     true,
	"was":     true,
	"were":    true,
	"has":     true,
	"have":    true,
	"not":     true,
	"missing": true,
}
