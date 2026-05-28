package codeintel

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	braceClassRE      = regexp.MustCompile(`\b(?:class|interface|struct|enum|trait|object)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	rustImplRE        = regexp.MustCompile(`\bimpl(?:\s*<[^>]+>)?\s+([A-Za-z_][A-Za-z0-9_]*)`)
	jsFunctionRE      = regexp.MustCompile(`\b(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	jsArrowFunctionRE = regexp.MustCompile(`\b(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?(?:function\b|\([^)]*\)\s*=>|[A-Za-z_$][A-Za-z0-9_$]*\s*=>)`)
	braceMethodRE     = regexp.MustCompile(`^\s*(?:public|private|protected|static|async|override|final|abstract|export|default|readonly|virtual|inline|constexpr|const|mut|pub|suspend|open|internal|external|unsafe|\s)*([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^;{}]*\)\s*(?::\s*[^{}]+)?(?:\s+throws\s+[^{]+)?\s*(?:\{|$)`)
	rustFunctionRE    = regexp.MustCompile(`\b(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:<[^>]+>)?\s*\(`)
	cFamilyFunctionRE = regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|final|abstract|override|virtual|inline|constexpr|extern|export|async|unsafe|suspend|open|internal|friend|template\s*<[^>]+>)\s+)*[A-Za-z_~][A-Za-z0-9_:<>\[\],*&?\s.]*\s+([A-Za-z_~][A-Za-z0-9_]*)\s*\([^;{}]*\)\s*(?:const\s*)?(?:throws\s+[^{]+)?(?:\{|$)`)
	phpFunctionRE     = regexp.MustCompile(`\bfunction\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	pythonClassRE     = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b.*:\s*(?:#.*)?$`)
	pythonFunctionRE  = regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	rubyClassRE       = regexp.MustCompile(`^\s*(?:class|module)\s+([A-Za-z_][A-Za-z0-9_:]*)\b`)
	rubyFunctionRE    = regexp.MustCompile(`^\s*def\s+(?:self\.)?([A-Za-z_][A-Za-z0-9_!?=]*)\b`)
)

func DetectLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py", ".pyw":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".cs":
		return "csharp"
	case ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	default:
		return ""
	}
}

func LanguageLabel(language string) string {
	switch language {
	case "go":
		return "Go"
	case "typescript":
		return "TypeScript"
	case "javascript":
		return "JavaScript"
	case "python":
		return "Python"
	case "rust":
		return "Rust"
	case "java":
		return "Java"
	case "kotlin":
		return "Kotlin"
	case "csharp":
		return "C#"
	case "cpp":
		return "C/C++"
	case "ruby":
		return "Ruby"
	case "php":
		return "PHP"
	default:
		return strings.TrimSpace(language)
	}
}

func resolveHeuristicEnclosingSymbol(repoRoot string, targetPath string, line int64, language string) (Symbol, bool) {
	if line <= 0 {
		return Symbol{}, false
	}
	absPath, relPath, ok := sourceFilePath(repoRoot, targetPath)
	if !ok {
		return Symbol{}, false
	}
	source, ok := readSource(absPath)
	if !ok {
		return Symbol{}, false
	}
	symbols := heuristicSymbolsForFile(source, relPath, language)
	var (
		best     Symbol
		bestSpan int64
		found    bool
	)
	for _, symbol := range symbols {
		if line < symbol.StartLine || line > symbol.EndLine {
			continue
		}
		span := symbol.EndLine - symbol.StartLine
		if !found || span < bestSpan {
			best = symbol
			bestSpan = span
			found = true
		}
	}
	return best, found
}

func findHeuristicCallers(repoRoot string, target Symbol, limit int) []CallSite {
	if strings.TrimSpace(repoRoot) == "" || target.Name == "" || target.Language == "" || limit == 0 {
		return nil
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil
	}
	root = filepath.Clean(root)
	callers := []CallSite{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if skipGoDir(entry.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if DetectLanguage(path) != target.Language {
			return nil
		}
		if limit > 0 && len(callers) >= limit*4 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || isParentRelativePath(rel) {
			return nil
		}
		source, ok := readSource(path)
		if !ok {
			return nil
		}
		relPath := filepath.ToSlash(rel)
		callers = append(callers, findHeuristicCallersInFile(root, relPath, source, target)...)
		return nil
	})
	sort.SliceStable(callers, func(i, j int) bool {
		leftTest := sourcePathLooksTest(callers[i].Path)
		rightTest := sourcePathLooksTest(callers[j].Path)
		if leftTest != rightTest {
			return !leftTest
		}
		leftSameDir := filepath.Dir(filepath.FromSlash(callers[i].Path)) == filepath.Dir(filepath.FromSlash(target.Path))
		rightSameDir := filepath.Dir(filepath.FromSlash(callers[j].Path)) == filepath.Dir(filepath.FromSlash(target.Path))
		if leftSameDir != rightSameDir {
			return leftSameDir
		}
		if callers[i].Path != callers[j].Path {
			return callers[i].Path < callers[j].Path
		}
		return callers[i].Line < callers[j].Line
	})
	callers = dedupeCallers(callers, target)
	if limit > 0 && len(callers) > limit {
		return callers[:limit]
	}
	return callers
}

func findHeuristicCallersInFile(repoRoot string, relPath string, source []byte, target Symbol) []CallSite {
	lines := splitSourceLines(source)
	callers := []CallSite{}
	for index, rawLine := range lines {
		lineNumber := int64(index + 1)
		if !lineContainsCall(rawLine, target.Name) || lineDeclaresSymbol(rawLine, target.Name, target.Language) {
			continue
		}
		caller, ok := resolveHeuristicEnclosingSymbol(repoRoot, relPath, lineNumber, target.Language)
		if !ok || caller.Name == "" {
			continue
		}
		column := strings.Index(rawLine, target.Name)
		if column < 0 {
			column = 0
		}
		callers = append(callers, CallSite{
			Caller: caller,
			Path:   relPath,
			Line:   lineNumber,
			Column: column + 1,
		})
	}
	return callers
}

func heuristicSymbolsForFile(source []byte, relPath string, language string) []Symbol {
	switch language {
	case "python":
		return pythonSymbolsForFile(source, relPath)
	case "ruby":
		return rubySymbolsForFile(source, relPath)
	default:
		return braceLanguageSymbolsForFile(source, relPath, language)
	}
}

func braceLanguageSymbolsForFile(source []byte, relPath string, language string) []Symbol {
	lines := splitSourceLines(source)
	containers := braceContainers(lines, relPath, language)
	symbols := []Symbol{}
	for index, line := range lines {
		name, kind, ok := braceFunctionName(line, language)
		if !ok || controlKeyword(name) {
			continue
		}
		startLine := int64(index + 1)
		endLine := braceBlockEnd(lines, index)
		receiver := nearestContainer(containers, startLine)
		qualified := name
		if receiver != "" && receiver != name {
			qualified = receiver + "." + name
			kind = "method"
		}
		symbols = append(symbols, Symbol{
			Name:          name,
			QualifiedName: qualified,
			Receiver:      receiver,
			Kind:          kind,
			Language:      language,
			Path:          relPath,
			StartLine:     startLine,
			EndLine:       endLine,
			NameLine:      startLine,
			NameColumn:    symbolNameColumn(line, name),
			Provenance:    "heuristic_source_scan",
		})
	}
	return symbols
}

func braceContainers(lines []string, relPath string, language string) []Symbol {
	containers := []Symbol{}
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		name := ""
		switch {
		case language == "rust":
			if match := rustImplRE.FindStringSubmatch(trimmed); len(match) == 2 {
				name = match[1]
			}
		}
		if name == "" {
			if match := braceClassRE.FindStringSubmatch(trimmed); len(match) == 2 {
				name = match[1]
			}
		}
		if name == "" {
			continue
		}
		startLine := int64(index + 1)
		containers = append(containers, Symbol{
			Name:          name,
			QualifiedName: name,
			Kind:          "type",
			Language:      language,
			Path:          relPath,
			StartLine:     startLine,
			EndLine:       braceBlockEnd(lines, index),
			NameLine:      startLine,
			NameColumn:    symbolNameColumn(line, name),
			Provenance:    "heuristic_source_scan",
		})
	}
	return containers
}

func braceFunctionName(line string, language string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
		return "", "", false
	}
	for _, expr := range []*regexp.Regexp{jsFunctionRE, jsArrowFunctionRE, rustFunctionRE, phpFunctionRE} {
		if match := expr.FindStringSubmatch(trimmed); len(match) == 2 {
			return match[1], "function", true
		}
	}
	if language == "typescript" || language == "javascript" {
		if match := braceMethodRE.FindStringSubmatch(trimmed); len(match) == 2 {
			return match[1], "function", true
		}
		return "", "", false
	}
	if match := cFamilyFunctionRE.FindStringSubmatch(trimmed); len(match) == 2 {
		return match[1], "function", true
	}
	if match := braceMethodRE.FindStringSubmatch(trimmed); len(match) == 2 {
		return match[1], "function", true
	}
	return "", "", false
}

func pythonSymbolsForFile(source []byte, relPath string) []Symbol {
	lines := splitSourceLines(source)
	classes := indentationContainers(lines, relPath, "python", pythonClassRE, "type")
	symbols := []Symbol{}
	for index, line := range lines {
		match := pythonFunctionRE.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		name := match[1]
		startLine := int64(index + 1)
		endLine := indentationBlockEnd(lines, index)
		receiver := nearestContainer(classes, startLine)
		qualified := name
		kind := "function"
		if receiver != "" {
			qualified = receiver + "." + name
			kind = "method"
		}
		symbols = append(symbols, Symbol{
			Name:          name,
			QualifiedName: qualified,
			Receiver:      receiver,
			Kind:          kind,
			Language:      "python",
			Path:          relPath,
			StartLine:     startLine,
			EndLine:       endLine,
			NameLine:      startLine,
			NameColumn:    symbolNameColumn(line, name),
			Provenance:    "heuristic_source_scan",
		})
	}
	return symbols
}

func rubySymbolsForFile(source []byte, relPath string) []Symbol {
	lines := splitSourceLines(source)
	classes := rubyContainers(lines, relPath)
	symbols := []Symbol{}
	for index, line := range lines {
		match := rubyFunctionRE.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		name := match[1]
		startLine := int64(index + 1)
		endLine := rubyBlockEnd(lines, index)
		receiver := nearestContainer(classes, startLine)
		qualified := name
		kind := "function"
		if receiver != "" {
			qualified = receiver + "." + name
			kind = "method"
		}
		symbols = append(symbols, Symbol{
			Name:          name,
			QualifiedName: qualified,
			Receiver:      receiver,
			Kind:          kind,
			Language:      "ruby",
			Path:          relPath,
			StartLine:     startLine,
			EndLine:       endLine,
			NameLine:      startLine,
			NameColumn:    symbolNameColumn(line, name),
			Provenance:    "heuristic_source_scan",
		})
	}
	return symbols
}

func indentationContainers(lines []string, relPath string, language string, expr *regexp.Regexp, kind string) []Symbol {
	containers := []Symbol{}
	for index, line := range lines {
		match := expr.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		startLine := int64(index + 1)
		containers = append(containers, Symbol{
			Name:          match[1],
			QualifiedName: match[1],
			Kind:          kind,
			Language:      language,
			Path:          relPath,
			StartLine:     startLine,
			EndLine:       indentationBlockEnd(lines, index),
			NameLine:      startLine,
			NameColumn:    symbolNameColumn(line, match[1]),
			Provenance:    "heuristic_source_scan",
		})
	}
	return containers
}

func rubyContainers(lines []string, relPath string) []Symbol {
	containers := []Symbol{}
	for index, line := range lines {
		match := rubyClassRE.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		startLine := int64(index + 1)
		containers = append(containers, Symbol{
			Name:          match[1],
			QualifiedName: match[1],
			Kind:          "type",
			Language:      "ruby",
			Path:          relPath,
			StartLine:     startLine,
			EndLine:       rubyBlockEnd(lines, index),
			NameLine:      startLine,
			NameColumn:    symbolNameColumn(line, match[1]),
			Provenance:    "heuristic_source_scan",
		})
	}
	return containers
}

func nearestContainer(containers []Symbol, line int64) string {
	var (
		best     Symbol
		bestSpan int64
		found    bool
	)
	for _, container := range containers {
		if line < container.StartLine || line > container.EndLine {
			continue
		}
		span := container.EndLine - container.StartLine
		if !found || span < bestSpan {
			best = container
			bestSpan = span
			found = true
		}
	}
	return best.Name
}

func braceBlockEnd(lines []string, startIndex int) int64 {
	depth := 0
	seenOpen := false
	for index := startIndex; index < len(lines); index++ {
		for _, char := range stripLineStringLiterals(lines[index]) {
			switch char {
			case '{':
				depth++
				seenOpen = true
			case '}':
				if seenOpen {
					depth--
				}
			}
		}
		if seenOpen && depth <= 0 {
			return int64(index + 1)
		}
	}
	return int64(len(lines))
}

func indentationBlockEnd(lines []string, startIndex int) int64 {
	startIndent := lineIndent(lines[startIndex])
	end := len(lines)
	for index := startIndex + 1; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if lineIndent(lines[index]) <= startIndent {
			end = index
			break
		}
	}
	return int64(end)
}

func rubyBlockEnd(lines []string, startIndex int) int64 {
	depth := 0
	for index := startIndex; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "class ") || strings.HasPrefix(line, "module ") || strings.HasPrefix(line, "def ") || strings.HasSuffix(line, " do") {
			depth++
		}
		if line == "end" || strings.HasPrefix(line, "end ") {
			depth--
			if depth <= 0 {
				return int64(index + 1)
			}
		}
	}
	return int64(len(lines))
}

func lineContainsCall(line string, name string) bool {
	expr := regexp.MustCompile(`(?:^|[^A-Za-z0-9_$])` + regexp.QuoteMeta(name) + `\s*\(`)
	return expr.FindStringIndex(line) != nil
}

func lineDeclaresSymbol(line string, name string, language string) bool {
	switch language {
	case "python":
		match := pythonFunctionRE.FindStringSubmatch(line)
		return len(match) == 2 && match[1] == name
	case "ruby":
		match := rubyFunctionRE.FindStringSubmatch(line)
		return len(match) == 2 && match[1] == name
	default:
		symbolName, _, ok := braceFunctionName(line, language)
		return ok && symbolName == name
	}
}

func sourceFilePath(repoRoot string, targetPath string) (string, string, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	targetPath = strings.TrimSpace(targetPath)
	if repoRoot == "" || targetPath == "" || DetectLanguage(targetPath) == "" {
		return "", "", false
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", "", false
	}
	root = filepath.Clean(root)
	var absPath string
	if filepath.IsAbs(targetPath) {
		absPath = filepath.Clean(targetPath)
	} else {
		rel := filepath.Clean(filepath.FromSlash(targetPath))
		if isParentRelativePath(rel) {
			return "", "", false
		}
		absPath = filepath.Join(root, rel)
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil || isParentRelativePath(rel) {
		return "", "", false
	}
	return absPath, filepath.ToSlash(rel), true
}

func readSource(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxGoSourceBytes {
		return nil, false
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return source, true
}

func splitSourceLines(source []byte) []string {
	return strings.Split(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n")
}

func symbolNameColumn(line string, name string) int {
	index := strings.Index(line, name)
	if index < 0 {
		return 1
	}
	return index + 1
}

func lineIndent(line string) int {
	indent := 0
	for _, char := range line {
		switch char {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}

func stripLineStringLiterals(line string) string {
	var builder strings.Builder
	inString := rune(0)
	escaped := false
	for _, char := range line {
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == inString {
				inString = 0
			}
			continue
		}
		if char == '"' || char == '\'' || char == '`' {
			inString = char
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func controlKeyword(name string) bool {
	switch name {
	case "if", "for", "while", "switch", "catch", "with", "return", "sizeof", "new", "delete", "else", "do":
		return true
	default:
		return false
	}
}

func sourcePathLooksTest(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(base, "test") || strings.Contains(path, "/test/") || strings.Contains(path, "/tests/") || strings.Contains(path, "__tests__")
}

func dedupeCallers(callers []CallSite, target Symbol) []CallSite {
	seen := map[string]struct{}{}
	deduped := make([]CallSite, 0, len(callers))
	for _, caller := range callers {
		if caller.Caller.Path == target.Path && caller.Caller.NameLine == target.NameLine {
			continue
		}
		key := caller.Caller.QualifiedName + "|" + caller.Path + "|" + int64Key(caller.Line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, caller)
	}
	return deduped
}
