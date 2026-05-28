package codeintel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGoEnclosingSymbolFindsFunctionFromBodyLine(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoIntelFile(t, repoRoot, "internal/prices.go", `package internal

func pickTokenPrice(prices *[2]*float64) float64 {
	if prices == nil {
		return 0
	}
	return *prices[0]
}
`)

	symbol, ok := ResolveGoEnclosingSymbol(repoRoot, "internal/prices.go", 5)
	if !ok {
		t.Fatal("ResolveGoEnclosingSymbol() ok = false")
	}
	if symbol.Name != "pickTokenPrice" ||
		symbol.QualifiedName != "pickTokenPrice" ||
		symbol.Kind != "function" ||
		symbol.NameLine != 3 ||
		symbol.NameColumn <= 0 ||
		symbol.Provenance != "go_ast" {
		t.Fatalf("symbol = %+v", symbol)
	}
}

func TestResolveGoEnclosingSymbolFindsMethodOnBlankLine(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoIntelFile(t, repoRoot, "internal/service.go", `package internal

type Service struct{}

func (s *Service) UpdateSettings() {
	if s == nil {
		return
	}

	return
}
`)

	symbol, ok := ResolveGoEnclosingSymbol(repoRoot, "internal/service.go", 8)
	if !ok {
		t.Fatal("ResolveGoEnclosingSymbol() ok = false")
	}
	if symbol.Name != "UpdateSettings" ||
		symbol.QualifiedName != "Service.UpdateSettings" ||
		symbol.Receiver != "Service" ||
		symbol.Kind != "method" ||
		symbol.StartLine != 5 ||
		symbol.EndLine != 11 {
		t.Fatalf("symbol = %+v", symbol)
	}
}

func TestResolveGoEnclosingSymbolUsesParentForClosureLine(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoIntelFile(t, repoRoot, "internal/jobs.go", `package internal

func RunJob() {
	_ = func() bool {
		return true
	}()
}
`)

	symbol, ok := ResolveGoEnclosingSymbol(repoRoot, "internal/jobs.go", 5)
	if !ok {
		t.Fatal("ResolveGoEnclosingSymbol() ok = false")
	}
	if symbol.QualifiedName != "RunJob" {
		t.Fatalf("symbol = %+v, want parent RunJob", symbol)
	}
}

func TestFindGoCallersRanksProductionCallersBeforeTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoIntelFile(t, repoRoot, "internal/prices.go", `package internal

func pickTokenPrice(prices *[2]*float64) float64 {
	return 0
}
`)
	writeGoIntelFile(t, repoRoot, "internal/fetcher.go", `package internal

func fetchRewardTokenInfo(prices *[2]*float64) float64 {
	return pickTokenPrice(prices)
}
`)
	writeGoIntelFile(t, repoRoot, "internal/prices_test.go", `package internal

func TestPickTokenPrice() {
	_ = pickTokenPrice(nil)
}
`)

	symbol, ok := ResolveGoEnclosingSymbol(repoRoot, "internal/prices.go", 3)
	if !ok {
		t.Fatal("ResolveGoEnclosingSymbol() ok = false")
	}
	callers := FindGoCallers(repoRoot, symbol, 4)
	if len(callers) != 2 {
		t.Fatalf("callers len = %d, want 2: %+v", len(callers), callers)
	}
	if callers[0].Caller.QualifiedName != "fetchRewardTokenInfo" ||
		callers[0].Path != "internal/fetcher.go" ||
		callers[0].Line != 4 {
		t.Fatalf("first caller = %+v", callers[0])
	}
	if callers[1].Path != "internal/prices_test.go" {
		t.Fatalf("second caller = %+v", callers[1])
	}
}

func TestResolveEnclosingSymbolFindsTypeScriptFunctionAndCaller(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoIntelFile(t, repoRoot, "src/prices.ts", `export function pickTokenPrice(prices: number[]): number {
  if (!prices.length) {
    return 0
  }
  return prices[0]
}

export class RewardFetcher {
  fetchRewardTokenInfo(prices: number[]) {
    return pickTokenPrice(prices)
  }
}
`)

	symbol, ok := ResolveEnclosingSymbol(repoRoot, "src/prices.ts", 4)
	if !ok {
		t.Fatal("ResolveEnclosingSymbol() ok = false")
	}
	if symbol.Name != "pickTokenPrice" ||
		symbol.QualifiedName != "pickTokenPrice" ||
		symbol.Language != "typescript" ||
		symbol.Provenance != "heuristic_source_scan" {
		t.Fatalf("symbol = %+v", symbol)
	}
	callers := FindCallers(repoRoot, symbol, 4)
	if len(callers) != 1 {
		t.Fatalf("callers len = %d, want 1: %+v", len(callers), callers)
	}
	if callers[0].Caller.QualifiedName != "RewardFetcher.fetchRewardTokenInfo" ||
		callers[0].Path != "src/prices.ts" ||
		callers[0].Line != 10 {
		t.Fatalf("caller = %+v", callers[0])
	}
}

func TestResolveEnclosingSymbolFindsPythonMethodAndCaller(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoIntelFile(t, repoRoot, "app/prices.py", `def pick_token_price(prices):
    if not prices:
        return 0
    return prices[0]

class RewardFetcher:
    def fetch_reward_token_info(self, prices):
        return pick_token_price(prices)
`)

	symbol, ok := ResolveEnclosingSymbol(repoRoot, "app/prices.py", 3)
	if !ok {
		t.Fatal("ResolveEnclosingSymbol() ok = false")
	}
	if symbol.Name != "pick_token_price" ||
		symbol.Language != "python" ||
		symbol.Provenance != "heuristic_source_scan" {
		t.Fatalf("symbol = %+v", symbol)
	}
	callers := FindCallers(repoRoot, symbol, 4)
	if len(callers) != 1 {
		t.Fatalf("callers len = %d, want 1: %+v", len(callers), callers)
	}
	if callers[0].Caller.QualifiedName != "RewardFetcher.fetch_reward_token_info" ||
		callers[0].Path != "app/prices.py" ||
		callers[0].Line != 8 {
		t.Fatalf("caller = %+v", callers[0])
	}
}

func writeGoIntelFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
