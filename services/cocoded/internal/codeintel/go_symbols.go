package codeintel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const maxGoSourceBytes = 512 * 1024

type Symbol struct {
	Name          string
	QualifiedName string
	Receiver      string
	Kind          string
	Language      string
	Path          string
	StartLine     int64
	EndLine       int64
	NameLine      int64
	NameColumn    int
	Provenance    string
}

type CallSite struct {
	Caller Symbol
	Path   string
	Line   int64
	Column int
}

type GoSymbol = Symbol
type GoCallSite = CallSite

// ResolveEnclosingSymbol is the language-dispatch entry point for evidence-map
// enrichment. Add bundled tree-sitter or LSP-backed engines here, ahead of the
// heuristic fallback, when those parsers are packaged with cocode.
func ResolveEnclosingSymbol(repoRoot string, targetPath string, line int64) (Symbol, bool) {
	language := DetectLanguage(targetPath)
	if language == "" {
		return Symbol{}, false
	}
	if language == "go" {
		return ResolveGoEnclosingSymbol(repoRoot, targetPath, line)
	}
	return resolveHeuristicEnclosingSymbol(repoRoot, targetPath, line, language)
}

// FindCallers mirrors ResolveEnclosingSymbol: precise language engines should
// run first, then the bundled heuristic scanner supplies a no-dependency floor.
func FindCallers(repoRoot string, target Symbol, limit int) []CallSite {
	if target.Language == "go" || target.Provenance == "go_ast" {
		return FindGoCallers(repoRoot, target, limit)
	}
	return findHeuristicCallers(repoRoot, target, limit)
}

func ResolveGoEnclosingSymbol(repoRoot string, targetPath string, line int64) (GoSymbol, bool) {
	if line <= 0 {
		return GoSymbol{}, false
	}
	absPath, relPath, ok := goFilePath(repoRoot, targetPath)
	if !ok {
		return GoSymbol{}, false
	}
	source, ok := readGoSource(absPath)
	if !ok {
		return GoSymbol{}, false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, source, 0)
	if err != nil {
		return GoSymbol{}, false
	}
	var (
		bestDecl *ast.FuncDecl
		bestSpan int
	)
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl == nil || funcDecl.Name == nil {
			continue
		}
		start := fset.Position(funcDecl.Pos()).Line
		end := fset.Position(funcDecl.End()).Line
		if line < int64(start) || line > int64(end) {
			continue
		}
		span := end - start
		if bestDecl == nil || span < bestSpan {
			bestDecl = funcDecl
			bestSpan = span
		}
	}
	if bestDecl == nil {
		return GoSymbol{}, false
	}
	symbol, ok := goSymbolFromDecl(fset, relPath, bestDecl)
	if !ok {
		return GoSymbol{}, false
	}
	return symbol, true
}

func FindGoCallers(repoRoot string, target GoSymbol, limit int) []GoCallSite {
	if strings.TrimSpace(repoRoot) == "" || target.Name == "" || limit == 0 {
		return nil
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil
	}
	root = filepath.Clean(root)
	capacity := 0
	if limit > 0 {
		capacity = limit
	}
	callers := make([]GoCallSite, 0, capacity)
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
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		if limit > 0 && len(callers) >= limit*3 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || isParentRelativePath(rel) {
			return nil
		}
		source, ok := readGoSource(path)
		if !ok {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			return nil
		}
		relPath := filepath.ToSlash(rel)
		callers = append(callers, findGoCallersInFile(fset, relPath, file, target)...)
		return nil
	})
	sort.SliceStable(callers, func(i, j int) bool {
		leftTest := strings.HasSuffix(callers[i].Path, "_test.go")
		rightTest := strings.HasSuffix(callers[j].Path, "_test.go")
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
	callers = dedupeGoCallers(callers, target)
	if limit > 0 && len(callers) > limit {
		return callers[:limit]
	}
	return callers
}

func findGoCallersInFile(fset *token.FileSet, relPath string, file *ast.File, target GoSymbol) []GoCallSite {
	callers := []GoCallSite{}
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl == nil || funcDecl.Body == nil || funcDecl.Name == nil {
			continue
		}
		caller, ok := goSymbolFromDecl(fset, relPath, funcDecl)
		if !ok {
			continue
		}
		ast.Inspect(funcDecl.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || call == nil || !goCallMatchesTarget(call.Fun, target) {
				return true
			}
			position := fset.Position(call.Pos())
			callers = append(callers, GoCallSite{
				Caller: caller,
				Path:   relPath,
				Line:   int64(position.Line),
				Column: position.Column,
			})
			return true
		})
	}
	return callers
}

func goCallMatchesTarget(expr ast.Expr, target GoSymbol) bool {
	name := goCallName(expr)
	if name == "" || name != target.Name {
		return false
	}
	return true
}

func goCallName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		if value.Sel == nil {
			return ""
		}
		return value.Sel.Name
	case *ast.IndexExpr:
		return goCallName(value.X)
	case *ast.IndexListExpr:
		return goCallName(value.X)
	case *ast.ParenExpr:
		return goCallName(value.X)
	default:
		return ""
	}
}

func dedupeGoCallers(callers []GoCallSite, target GoSymbol) []GoCallSite {
	seen := map[string]struct{}{}
	deduped := make([]GoCallSite, 0, len(callers))
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

func goSymbolFromDecl(fset *token.FileSet, relPath string, decl *ast.FuncDecl) (GoSymbol, bool) {
	if decl == nil || decl.Name == nil {
		return GoSymbol{}, false
	}
	namePosition := fset.Position(decl.Name.Pos())
	startPosition := fset.Position(decl.Pos())
	endPosition := fset.Position(decl.End())
	if namePosition.Line <= 0 || namePosition.Column <= 0 || startPosition.Line <= 0 || endPosition.Line <= 0 {
		return GoSymbol{}, false
	}
	receiver := goReceiverName(decl.Recv)
	qualified := decl.Name.Name
	kind := "function"
	if receiver != "" {
		qualified = receiver + "." + decl.Name.Name
		kind = "method"
	}
	return GoSymbol{
		Name:          decl.Name.Name,
		QualifiedName: qualified,
		Receiver:      receiver,
		Kind:          kind,
		Language:      "go",
		Path:          filepath.ToSlash(relPath),
		StartLine:     int64(startPosition.Line),
		EndLine:       int64(endPosition.Line),
		NameLine:      int64(namePosition.Line),
		NameColumn:    namePosition.Column,
		Provenance:    "go_ast",
	}, true
}

func goReceiverName(receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) == 0 {
		return ""
	}
	return goExprName(receiver.List[0].Type)
}

func goExprName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return goExprName(value.X)
	case *ast.SelectorExpr:
		left := goExprName(value.X)
		if left == "" {
			if value.Sel == nil {
				return ""
			}
			return value.Sel.Name
		}
		if value.Sel == nil {
			return left
		}
		return left + "." + value.Sel.Name
	case *ast.IndexExpr:
		return goExprName(value.X)
	case *ast.IndexListExpr:
		return goExprName(value.X)
	case *ast.ParenExpr:
		return goExprName(value.X)
	default:
		return ""
	}
}

func goFilePath(repoRoot string, targetPath string) (string, string, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	targetPath = strings.TrimSpace(targetPath)
	if repoRoot == "" || targetPath == "" || filepath.Ext(targetPath) != ".go" {
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

func readGoSource(path string) ([]byte, bool) {
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

func skipGoDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "vendor", "node_modules", "dist", "build", ".next":
		return true
	default:
		return false
	}
}

func isParentRelativePath(path string) bool {
	path = filepath.Clean(path)
	return path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func int64Key(value int64) string {
	return strconv.FormatInt(value, 10)
}
