package agentoutput

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestFakeJSONAgentMatchesReviewAgentOutputSchema(t *testing.T) {
	t.Parallel()

	output := runFakeAgent(t, "json-agent.sh", "review this fixture")
	parsed := ParseAuto(output)
	if !parsed.Structured || len(parsed.Documents) != 1 {
		t.Fatalf("parsed = %+v", parsed)
	}

	validateJSONSchema(t, "review-agent-output.schema.json", parsed.Documents[0])
}

func TestFindingCandidateSchema(t *testing.T) {
	t.Parallel()

	validCandidate := []byte(`{
		"schema_version": "finding-candidate/v1",
		"claim": "Repository settings updates can run without an admin permission check.",
		"category": "security",
		"severity": "high",
		"confidence": 0.91,
		"locations": [
			{
				"path": "apps/api/src/routes/repositories.ts",
				"start_line": 87,
				"end_line": 112,
				"side": "RIGHT",
				"valid": true
			}
		],
		"primary_path": "apps/api/src/routes/repositories.ts",
		"primary_start_line": 87,
		"primary_end_line": 112,
		"evidence": [
			{
				"title": "Mutation route reaches updateSettings after member authentication only",
				"summary": "The route updates repository settings without requiring workspace admin privileges.",
				"kind": "changed_code",
				"path": "apps/api/src/routes/repositories.ts",
				"start_line": 87,
				"end_line": 112,
				"confidence": 0.9
			}
		],
		"counter_evidence_request": "Show an upstream admin-only guard that always runs before this route.",
		"suggested_fix": "Mount requireWorkspaceAdmin before updateRepositorySettings and add a member-denied regression test.",
		"draft_comment": "Please require workspace admin permission before mutating repository settings.",
		"fingerprint": "security:apps/api/src/routes/repositories.ts:87"
	}`)
	validateJSONSchema(t, "finding-candidate.schema.json", validCandidate)

	invalidSeverity := bytes.Replace(validCandidate, []byte(`"severity": "high"`), []byte(`"severity": "critical"`), 1)
	assertInvalidJSONSchema(t, "finding-candidate.schema.json", invalidSeverity)

	invalidConfidence := bytes.Replace(validCandidate, []byte(`"confidence": 0.91`), []byte(`"confidence": 1.2`), 1)
	assertInvalidJSONSchema(t, "finding-candidate.schema.json", invalidConfidence)
}

func TestParserKnownEnumsMatchPackageSchemas(t *testing.T) {
	t.Parallel()

	for _, schemaName := range []string{"review-agent-output.schema.json", "finding-candidate.schema.json"} {
		schema := readTestSchema(t, schemaName)
		assertStringSlicesEqual(t, schemaName+" category", KnownCategories(), schemaEnum(t, schema, "category"))
		assertStringSlicesEqual(t, schemaName+" severity", KnownSeverities(), schemaEnum(t, schema, "severity"))
		assertStringSlicesEqual(t, schemaName+" evidence_kind", KnownEvidenceKinds(), schemaEnum(t, schema, "evidence_kind"))
		assertStringSlicesEqual(t, schemaName+" location side", KnownSides(), schemaNestedEnum(t, schema, "location", "side"))
	}
}

func validateJSONSchema(t *testing.T, schemaName string, document []byte) {
	t.Helper()

	schema := compileTestSchema(t, schemaName)
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		t.Fatalf("UnmarshalJSON(%s instance) error = %v", schemaName, err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("Validate(%s) error = %v", schemaName, err)
	}
}

func assertInvalidJSONSchema(t *testing.T, schemaName string, document []byte) {
	t.Helper()

	schema := compileTestSchema(t, schemaName)
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		t.Fatalf("UnmarshalJSON(%s invalid instance) error = %v", schemaName, err)
	}
	if err := schema.Validate(instance); err == nil {
		t.Fatalf("Validate(%s) succeeded for invalid instance", schemaName)
	}
}

func readTestSchema(t *testing.T, name string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(schemaPath(t, name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", name, err)
	}
	return schema
}

func schemaEnum(t *testing.T, schema map[string]any, defName string) []string {
	t.Helper()

	defs := schema["$defs"].(map[string]any)
	definition := defs[defName].(map[string]any)
	return interfaceStrings(definition["enum"].([]any))
}

func schemaNestedEnum(t *testing.T, schema map[string]any, defName string, propertyName string) []string {
	t.Helper()

	defs := schema["$defs"].(map[string]any)
	definition := defs[defName].(map[string]any)
	properties := definition["properties"].(map[string]any)
	property := properties[propertyName].(map[string]any)
	return interfaceStrings(property["enum"].([]any))
}

func interfaceStrings(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.(string))
	}
	return result
}

func assertStringSlicesEqual(t *testing.T, label string, got []string, want []string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func compileTestSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()

	schema, err := jsonschema.NewCompiler().Compile(schemaPath(t, name))
	if err != nil {
		t.Fatalf("Compile(%s) error = %v", name, err)
	}
	return schema
}

func schemaPath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "packages", "schemas", name)
}
