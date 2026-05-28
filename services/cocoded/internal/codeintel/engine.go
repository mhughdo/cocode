package codeintel

// Engine is the extension point for precise language intelligence.
//
// The current package dispatches to Go's bundled stdlib parser first and then
// to the no-dependency heuristic scanner. Future bundled tree-sitter grammars
// or LSP clients should implement this shape and be registered in
// ResolveEnclosingSymbol / FindCallers ahead of the heuristic fallback.
type Engine interface {
	Language() string
	ResolveEnclosingSymbol(repoRoot string, targetPath string, line int64) (Symbol, bool)
	FindCallers(repoRoot string, target Symbol, limit int) []CallSite
}
