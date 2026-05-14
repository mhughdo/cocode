package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

const modelCatalogCommandTimeout = 12 * time.Second
const modelCatalogCacheTTL = 10 * time.Minute

var sharedAgentModelCatalogCache agentModelCatalogCache

type AgentModelCatalogResponse struct {
	Provider      string                     `json:"provider"`
	ProviderLabel string                     `json:"provider_label"`
	Command       string                     `json:"command"`
	Available     bool                       `json:"available"`
	Source        string                     `json:"source"`
	Models        []AgentModelOptionResponse `json:"models"`
	CachedAt      string                     `json:"cached_at,omitempty"`
	ExpiresAt     string                     `json:"expires_at,omitempty"`
	Stale         bool                       `json:"stale,omitempty"`
	Refreshing    bool                       `json:"refreshing,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

type AgentModelOptionResponse struct {
	ID               string                         `json:"id"`
	Label            string                         `json:"label"`
	Provider         string                         `json:"provider"`
	ProviderLabel    string                         `json:"provider_label"`
	Source           string                         `json:"source"`
	Default          bool                           `json:"default"`
	ReasoningEfforts []AgentReasoningOptionResponse `json:"reasoning_efforts,omitempty"`
}

type AgentReasoningOptionResponse struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

type codexReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type agentModelCatalogCache struct {
	mu         sync.Mutex
	entries    []AgentModelCatalogResponse
	cachedAt   time.Time
	expiresAt  time.Time
	refreshing bool
}

func listAgentModelCatalogHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		force := c.Query("refresh") == "1" || strings.EqualFold(c.Query("refresh"), "true")
		respondOK(c, sharedAgentModelCatalogCache.catalogs(c.Request.Context(), force))
	}
}

func (cache *agentModelCatalogCache) catalogs(ctx context.Context, force bool) []AgentModelCatalogResponse {
	now := time.Now()
	cache.mu.Lock()
	if !force && len(cache.entries) > 0 && now.Before(cache.expiresAt) {
		out := catalogSnapshot(cache.entries)
		cachedAt := cache.cachedAt
		expiresAt := cache.expiresAt
		cache.mu.Unlock()
		return withCatalogCacheMetadata(out, cachedAt, expiresAt, false, false)
	}
	if !force && len(cache.entries) > 0 {
		out := catalogSnapshot(cache.entries)
		cachedAt := cache.cachedAt
		expiresAt := cache.expiresAt
		refreshing := cache.refreshing
		if !cache.refreshing {
			cache.refreshing = true
			refreshing = true
			go cache.refresh(context.Background())
		}
		cache.mu.Unlock()
		return withCatalogCacheMetadata(out, cachedAt, expiresAt, true, refreshing)
	}
	if cache.refreshing && len(cache.entries) > 0 {
		out := catalogSnapshot(cache.entries)
		cachedAt := cache.cachedAt
		expiresAt := cache.expiresAt
		cache.mu.Unlock()
		return withCatalogCacheMetadata(out, cachedAt, expiresAt, true, true)
	}
	cache.refreshing = true
	cache.mu.Unlock()

	return cache.refresh(ctx)
}

func (cache *agentModelCatalogCache) refresh(ctx context.Context) []AgentModelCatalogResponse {
	catalogs := discoverAgentModelCatalogs(ctx)
	now := time.Now()
	expiresAt := now.Add(modelCatalogCacheTTL)

	cache.mu.Lock()
	cache.entries = catalogSnapshot(catalogs)
	cache.cachedAt = now
	cache.expiresAt = expiresAt
	cache.refreshing = false
	cache.mu.Unlock()

	return withCatalogCacheMetadata(catalogs, now, expiresAt, false, false)
}

func discoverAgentModelCatalogs(ctx context.Context) []AgentModelCatalogResponse {
	discoverers := []func(context.Context) AgentModelCatalogResponse{
		discoverCodexModels,
		discoverOpenCodeModels,
		discoverKiroModels,
		discoverClaudeModels,
		discoverGeminiModels,
	}
	catalogs := make([]AgentModelCatalogResponse, len(discoverers))
	var wg sync.WaitGroup
	wg.Add(len(discoverers))
	for index, discover := range discoverers {
		go func(index int, discover func(context.Context) AgentModelCatalogResponse) {
			defer wg.Done()
			catalogs[index] = discover(ctx)
		}(index, discover)
	}
	wg.Wait()
	return catalogs
}

func discoverCodexModels(ctx context.Context) AgentModelCatalogResponse {
	catalog := modelCatalog("openai", "OpenAI", "codex")
	if _, err := exec.LookPath("codex"); err != nil {
		catalog.Error = "codex command is not installed or not on PATH"
		return catalog
	}
	catalog.Available = true
	stdout, stderr, err := runModelCatalogCommand(ctx, "codex", "debug", "models")
	if err != nil {
		catalog.Error = commandCatalogError(err, stderr)
		return catalog
	}

	var payload struct {
		Models []struct {
			Slug                     string                `json:"slug"`
			DisplayName              string                `json:"display_name"`
			Visibility               string                `json:"visibility"`
			DefaultReasoningLevel    string                `json:"default_reasoning_level"`
			SupportedReasoningLevels []codexReasoningLevel `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		catalog.Error = "codex model catalog was not valid JSON"
		return catalog
	}
	for _, model := range payload.Models {
		id := strings.TrimSpace(model.Slug)
		if id == "" || strings.EqualFold(strings.TrimSpace(model.Visibility), "hide") {
			continue
		}
		label := strings.TrimSpace(model.DisplayName)
		if label == "" {
			label = modelIDLabel(id)
		}
		catalog.Models = append(catalog.Models, AgentModelOptionResponse{
			ID:               id,
			Label:            label,
			Provider:         "openai",
			ProviderLabel:    providerLabel("openai"),
			Source:           "cli",
			ReasoningEfforts: codexReasoningOptions(model.DefaultReasoningLevel, model.SupportedReasoningLevels),
		})
	}
	markFirstDefault(catalog.Models)
	catalog.Source = sourceForModels(catalog.Models)
	return catalog
}

func discoverOpenCodeModels(ctx context.Context) AgentModelCatalogResponse {
	catalog := modelCatalog("opencode", "OpenCode", "opencode")
	if _, err := agents.ResolveCommandExecutable("opencode"); err != nil {
		catalog.Error = "opencode command is not installed or not on PATH"
		return catalog
	}
	catalog.Available = true
	stdout, stderr, err := runModelCatalogCommand(ctx, "opencode", "models", "--verbose")
	if err != nil {
		stdout, stderr, err = runModelCatalogCommand(ctx, "opencode", "models")
		if err != nil {
			catalog.Error = commandCatalogError(err, stderr)
			return catalog
		}
		catalog.Models = openCodePlainModelOptions(stdout)
		markPreferredDefault(catalog.Models, []string{"opencode-go/kimi-k2.6", "xai/grok-4.3", "xai/grok-4"})
		catalog.Source = sourceForModels(catalog.Models)
		return catalog
	}
	catalog.Models = openCodeVerboseModelOptions(stdout)
	if len(catalog.Models) == 0 {
		catalog.Models = openCodePlainModelOptions(stdout)
	}
	markPreferredDefault(catalog.Models, []string{"opencode-go/kimi-k2.6", "xai/grok-4.3", "xai/grok-4"})
	catalog.Source = sourceForModels(catalog.Models)
	return catalog
}

func openCodePlainModelOptions(stdout string) []AgentModelOptionResponse {
	models := []AgentModelOptionResponse{}
	for _, line := range strings.Split(stdout, "\n") {
		id := strings.TrimSpace(line)
		if id == "" || strings.HasPrefix(id, "#") {
			continue
		}
		provider := openCodeModelProvider(id)
		models = append(models, AgentModelOptionResponse{
			ID:               id,
			Label:            modelIDLabel(id),
			Provider:         provider,
			ProviderLabel:    providerLabel(provider),
			Source:           "cli",
			ReasoningEfforts: genericReasoningOptions("medium", []string{"low", "medium", "high"}),
		})
	}
	return models
}

func openCodeVerboseModelOptions(stdout string) []AgentModelOptionResponse {
	type verbosePayload struct {
		Name     string                     `json:"name"`
		Variants map[string]json.RawMessage `json:"variants"`
	}
	type candidate struct {
		id      string
		payload verbosePayload
	}
	candidates := []candidate{}
	lines := strings.Split(stdout, "\n")
	for index := 0; index < len(lines); index++ {
		id := strings.TrimSpace(lines[index])
		if id == "" || strings.HasPrefix(id, "{") || strings.HasPrefix(id, "}") || strings.HasPrefix(id, "#") {
			continue
		}
		if _, _, ok := strings.Cut(id, "/"); !ok {
			continue
		}
		jsonText, next := collectOpenCodeJSONBlock(lines, index+1)
		if jsonText == "" {
			continue
		}
		index = next - 1
		var payload verbosePayload
		if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
			continue
		}
		candidates = append(candidates, candidate{id: id, payload: payload})
	}
	models := make([]AgentModelOptionResponse, 0, len(candidates))
	for _, item := range candidates {
		provider := openCodeModelProvider(item.id)
		label := strings.TrimSpace(item.payload.Name)
		if label == "" {
			label = modelIDLabel(item.id)
		}
		models = append(models, AgentModelOptionResponse{
			ID:               item.id,
			Label:            label,
			Provider:         provider,
			ProviderLabel:    providerLabel(provider),
			Source:           "cli",
			ReasoningEfforts: openCodeReasoningOptions(item.payload.Variants),
		})
	}
	return models
}

func collectOpenCodeJSONBlock(lines []string, start int) (string, int) {
	var builder strings.Builder
	depth := 0
	started := false
	for index := start; index < len(lines); index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if !started {
			if !strings.HasPrefix(trimmed, "{") {
				continue
			}
			started = true
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
		depth += strings.Count(line, "{")
		depth -= strings.Count(line, "}")
		if started && depth <= 0 {
			return builder.String(), index + 1
		}
	}
	return "", len(lines)
}

func openCodeReasoningOptions(variants map[string]json.RawMessage) []AgentReasoningOptionResponse {
	if len(variants) == 0 {
		return nil
	}
	preferred := []string{"low", "medium", "high", "max", "minimal", "xhigh"}
	efforts := make([]string, 0, len(variants))
	seen := map[string]struct{}{}
	for _, effort := range preferred {
		if _, ok := variants[effort]; ok {
			efforts = append(efforts, effort)
			seen[effort] = struct{}{}
		}
	}
	for effort := range variants {
		key := strings.TrimSpace(effort)
		if key == "" {
			continue
		}
		lower := strings.ToLower(key)
		if _, ok := seen[lower]; ok {
			continue
		}
		efforts = append(efforts, key)
	}
	defaultEffort := ""
	if _, ok := variants["high"]; ok {
		defaultEffort = "high"
	} else if _, ok := variants["medium"]; ok {
		defaultEffort = "medium"
	}
	return genericReasoningOptions(defaultEffort, efforts)
}

func openCodeModelProvider(id string) string {
	provider := "opencode"
	if prefix, _, ok := strings.Cut(id, "/"); ok && strings.TrimSpace(prefix) != "" {
		provider = strings.TrimSpace(prefix)
	}
	return provider
}

func discoverClaudeModels(_ context.Context) AgentModelCatalogResponse {
	catalog := modelCatalog("anthropic", "Anthropic", "claude")
	if _, err := exec.LookPath("claude"); err != nil {
		catalog.Error = "claude command is not installed or not on PATH"
		return catalog
	}
	catalog.Available = true
	catalog.Models = knownModelOptions("anthropic", "cli-known", []knownModel{
		{ID: "sonnet", Label: "Sonnet", Default: true},
		{ID: "opus", Label: "Opus"},
		{ID: "claude-sonnet-4-6", Label: "Claude Sonnet 4.6"},
		{ID: "claude-opus-4-5", Label: "Claude Opus 4.5"},
		{ID: "claude-haiku-4-5", Label: "Claude Haiku 4.5"},
	}, genericReasoningOptions("high", []string{"low", "medium", "high", "xhigh", "max"}))
	catalog.Source = sourceForModels(catalog.Models)
	return catalog
}

func discoverGeminiModels(_ context.Context) AgentModelCatalogResponse {
	catalog := modelCatalog("google", "Google", "gemini")
	if _, err := exec.LookPath("gemini"); err != nil {
		catalog.Error = "gemini command is not installed or not on PATH"
		return catalog
	}
	catalog.Available = true
	catalog.Models = knownModelOptions("google", "cli-known", []knownModel{
		{ID: "gemini-3.1-pro-preview", Label: "Gemini 3.1 Pro Preview", Default: true},
		{ID: "gemini-2.5-pro", Label: "Gemini 2.5 Pro"},
		{ID: "gemini-2.5-flash", Label: "Gemini 2.5 Flash"},
		{ID: "default", Label: "CLI default"},
	}, nil)
	catalog.Source = sourceForModels(catalog.Models)
	return catalog
}

func discoverKiroModels(ctx context.Context) AgentModelCatalogResponse {
	catalog := modelCatalog("kiro", "Kiro", "kiro-cli")
	if _, err := agents.ResolveCommandExecutable("kiro-cli"); err != nil {
		catalog.Error = "kiro-cli command is not installed or not on PATH"
		return catalog
	}
	catalog.Available = true
	stdout, stderr, err := runModelCatalogCommand(ctx, "kiro-cli", "chat", "--list-models", "--format", "json")
	if err != nil {
		catalog.Error = commandCatalogError(err, stderr)
		return catalog
	}
	catalog.Models = kiroModelOptions(stdout)
	catalog.Source = sourceForModels(catalog.Models)
	return catalog
}

func kiroModelOptions(stdout string) []AgentModelOptionResponse {
	var payload struct {
		Models []struct {
			ModelName string `json:"model_name"`
			ModelID   string `json:"model_id"`
		} `json:"models"`
		DefaultModel string `json:"default_model"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		return nil
	}
	models := make([]AgentModelOptionResponse, 0, len(payload.Models))
	for _, model := range payload.Models {
		id := strings.TrimSpace(model.ModelID)
		if id == "" {
			id = strings.TrimSpace(model.ModelName)
		}
		if id == "" {
			continue
		}
		label := strings.TrimSpace(model.ModelName)
		if label == "" {
			label = modelIDLabel(id)
		}
		models = append(models, AgentModelOptionResponse{
			ID:            id,
			Label:         modelIDLabel(label),
			Provider:      "kiro",
			ProviderLabel: providerLabel("kiro"),
			Source:        "cli",
			Default:       strings.EqualFold(id, strings.TrimSpace(payload.DefaultModel)),
		})
	}
	markPreferredDefault(models, []string{payload.DefaultModel, "auto"})
	return models
}

type knownModel struct {
	ID      string
	Label   string
	Default bool
}

func knownModelOptions(provider string, source string, models []knownModel, reasoning []AgentReasoningOptionResponse) []AgentModelOptionResponse {
	options := make([]AgentModelOptionResponse, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		label := strings.TrimSpace(model.Label)
		if label == "" {
			label = modelIDLabel(id)
		}
		option := AgentModelOptionResponse{
			ID:            id,
			Label:         label,
			Provider:      provider,
			ProviderLabel: providerLabel(provider),
			Source:        source,
			Default:       model.Default,
		}
		if len(reasoning) > 0 {
			option.ReasoningEfforts = append([]AgentReasoningOptionResponse(nil), reasoning...)
		}
		options = append(options, option)
	}
	markFirstDefaultIfMissing(options)
	return options
}

func modelCatalog(provider string, providerLabel string, command string) AgentModelCatalogResponse {
	return AgentModelCatalogResponse{
		Provider:      provider,
		ProviderLabel: providerLabel,
		Command:       command,
		Source:        "unavailable",
		Models:        []AgentModelOptionResponse{},
	}
}

func catalogSnapshot(in []AgentModelCatalogResponse) []AgentModelCatalogResponse {
	out := make([]AgentModelCatalogResponse, 0, len(in))
	for _, catalog := range in {
		next := catalog
		next.Models = make([]AgentModelOptionResponse, 0, len(catalog.Models))
		for _, model := range catalog.Models {
			modelCopy := model
			modelCopy.ReasoningEfforts = append([]AgentReasoningOptionResponse(nil), model.ReasoningEfforts...)
			next.Models = append(next.Models, modelCopy)
		}
		out = append(out, next)
	}
	return out
}

func withCatalogCacheMetadata(catalogs []AgentModelCatalogResponse, cachedAt time.Time, expiresAt time.Time, stale bool, refreshing bool) []AgentModelCatalogResponse {
	out := catalogSnapshot(catalogs)
	cachedAtText := cachedAt.UTC().Format(time.RFC3339Nano)
	expiresAtText := expiresAt.UTC().Format(time.RFC3339Nano)
	for index := range out {
		out[index].CachedAt = cachedAtText
		out[index].ExpiresAt = expiresAtText
		out[index].Stale = stale
		out[index].Refreshing = refreshing
	}
	return out
}

func providerLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "OpenAI"
	case "opencode":
		return "OpenCode"
	case "opencode-go":
		return "OpenCode Go"
	case "anthropic":
		return "Anthropic"
	case "google":
		return "Google"
	case "kiro":
		return "Kiro"
	case "xai":
		return "xAI"
	case "openrouter":
		return "OpenRouter"
	case "azure":
		return "Azure"
	case "aws", "bedrock":
		return "AWS Bedrock"
	default:
		return modelIDLabel(provider)
	}
}

func runModelCatalogCommand(ctx context.Context, command string, args ...string) (string, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, modelCatalogCommandTimeout)
	defer cancel()
	env := currentProcessEnvMap()
	env = agents.NormalizeCLIEnvironment(command, env)
	executable, err := agents.ResolveCommandExecutableWithEnv(command, env)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(runCtx, executable, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = envListFromMap(env)
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

func currentProcessEnvMap() map[string]string {
	env := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	return env
}

func envListFromMap(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

func commandCatalogError(err error, stderr string) string {
	message := strings.TrimSpace(stderr)
	if message == "" && err != nil {
		message = err.Error()
	}
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}

func markFirstDefault(models []AgentModelOptionResponse) {
	if len(models) > 0 {
		models[0].Default = true
	}
}

func markFirstDefaultIfMissing(models []AgentModelOptionResponse) {
	if len(models) == 0 {
		return
	}
	for _, model := range models {
		if model.Default {
			return
		}
	}
	models[0].Default = true
}

func markPreferredDefault(models []AgentModelOptionResponse, preferred []string) {
	if len(models) == 0 {
		return
	}
	for _, id := range preferred {
		for index := range models {
			if strings.EqualFold(models[index].ID, id) {
				models[index].Default = true
				return
			}
		}
	}
	markFirstDefault(models)
}

func codexReasoningOptions(defaultEffort string, supported []codexReasoningLevel) []AgentReasoningOptionResponse {
	if len(supported) == 0 {
		return genericReasoningOptions(defaultEffort, []string{"low", "medium", "high", "xhigh"})
	}
	efforts := make([]string, 0, len(supported))
	for _, level := range supported {
		efforts = append(efforts, level.Effort)
	}
	return genericReasoningOptions(defaultEffort, efforts)
}

func genericReasoningOptions(defaultEffort string, efforts []string) []AgentReasoningOptionResponse {
	defaultEffort = strings.TrimSpace(defaultEffort)
	seen := map[string]struct{}{}
	options := make([]AgentReasoningOptionResponse, 0, len(efforts))
	for _, effort := range efforts {
		effort = strings.TrimSpace(effort)
		if effort == "" {
			continue
		}
		key := strings.ToLower(effort)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		options = append(options, AgentReasoningOptionResponse{
			ID:      effort,
			Label:   reasoningEffortLabel(effort),
			Default: strings.EqualFold(effort, defaultEffort),
		})
	}
	if len(options) == 0 {
		return nil
	}
	if defaultEffort == "" {
		defaultEffort = options[0].ID
	}
	hasDefault := false
	for index := range options {
		if strings.EqualFold(options[index].ID, defaultEffort) {
			options[index].Default = true
			hasDefault = true
		} else {
			options[index].Default = false
		}
	}
	if !hasDefault {
		options[0].Default = true
	}
	return options
}

func reasoningEffortLabel(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none":
		return "None"
	case "minimal":
		return "Minimal"
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "high":
		return "High"
	case "xhigh":
		return "Extra high"
	case "max":
		return "Max"
	default:
		return modelIDLabel(effort)
	}
}

func sourceForModels(models []AgentModelOptionResponse) string {
	for _, model := range models {
		if model.Source != "" {
			return model.Source
		}
	}
	return "unavailable"
}

func modelIDLabel(id string) string {
	_, slug, ok := strings.Cut(strings.TrimSpace(id), "/")
	if !ok {
		slug = strings.TrimSpace(id)
	}
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_'
	})
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch strings.ToLower(part) {
		case "gpt", "api", "cli", "json", "xai":
			labels = append(labels, strings.ToUpper(part))
		default:
			labels = append(labels, strings.ToUpper(part[:1])+part[1:])
		}
	}
	if len(labels) == 0 {
		return id
	}
	return strings.Join(labels, " ")
}
