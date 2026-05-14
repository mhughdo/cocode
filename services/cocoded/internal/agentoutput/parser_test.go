package agentoutput

import (
	"encoding/json"
	"testing"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

func TestParseJSONReturnsStructuredDocument(t *testing.T) {
	t.Parallel()

	parsed := Parse([]byte("  {\"findings\":[{\"claim\":\"bug\"}],\"count\":1}\n"), agents.OutputJSON)
	if !parsed.Structured ||
		parsed.Mode != agents.OutputJSON ||
		len(parsed.Documents) != 1 ||
		parsed.Text == "" ||
		len(parsed.Diagnostics) != 0 {
		t.Fatalf("parsed = %+v", parsed)
	}
	var decoded map[string]any
	if err := json.Unmarshal(parsed.Documents[0], &decoded); err != nil {
		t.Fatalf("Unmarshal(document) error = %v", err)
	}
	if decoded["count"].(float64) != 1 {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestParseInvalidJSONFallsBackToText(t *testing.T) {
	t.Parallel()

	raw := []byte("{\"findings\":[")
	parsed := Parse(raw, agents.OutputJSON)
	if parsed.Structured ||
		parsed.Mode != agents.OutputText ||
		parsed.Text != string(raw) ||
		len(parsed.Documents) != 0 ||
		len(parsed.Diagnostics) != 1 ||
		parsed.Diagnostics[0].Code != "invalid_json" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseJSONAutoDetectsDelimitedStream(t *testing.T) {
	t.Parallel()

	raw := []byte("{\"type\":\"thread.started\"}\n{\"type\":\"result\",\"result\":\"done\"}\n")
	parsed := Parse(raw, agents.OutputJSON)
	if !parsed.Structured ||
		parsed.Mode != agents.OutputJSONL ||
		len(parsed.Documents) != 2 ||
		len(parsed.Diagnostics) != 1 ||
		parsed.Diagnostics[0].Code != "output_mode_autodetected" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseJSONLReturnsDocuments(t *testing.T) {
	t.Parallel()

	raw := []byte("{\"event\":\"finding\",\"id\":1}\n\n{\"event\":\"done\",\"count\":1}\r\n")
	parsed := Parse(raw, agents.OutputJSONL)
	if !parsed.Structured ||
		parsed.Mode != agents.OutputJSONL ||
		len(parsed.Documents) != 2 ||
		parsed.Text != "" ||
		len(parsed.Diagnostics) != 0 {
		t.Fatalf("parsed = %+v", parsed)
	}
	assertJSONField(t, parsed.Documents[0], "event", "finding")
	assertJSONField(t, parsed.Documents[1], "event", "done")
}

func TestExtractCandidatesSkipsObjectShapedStreamEvents(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"partial"}}}
{"type":"result","result":"{\"findings\":[{\"claim\":\"missing guard\",\"category\":\"security\",\"severity\":\"high\",\"confidence\":0.8,\"locations\":[{\"path\":\"src/server.js\",\"start_line\":10,\"end_line\":10,\"side\":\"RIGHT\"}],\"evidence\":[{\"kind\":\"changed_code\",\"title\":\"unguarded call\",\"summary\":\"the handler calls the database without a guard\"}]}]}"}
`)
	parsed := Parse(raw, agents.OutputJSONL)
	result := ExtractCandidates(parsed)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Diagnostics = %+v", result.Diagnostics)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Claim != "missing guard" {
		t.Fatalf("Candidates = %+v", result.Candidates)
	}
}

func TestParseNDJSONAllowsMixedLogLines(t *testing.T) {
	t.Parallel()

	raw := []byte("starting agent\n{\"type\":\"finding\",\"claim\":\"unsafe path\"}\nnot json\n{\"type\":\"summary\",\"count\":1}\n")
	parsed := Parse(raw, agents.OutputNDJSON)
	if !parsed.Structured ||
		parsed.Mode != agents.OutputNDJSON ||
		len(parsed.Documents) != 2 ||
		parsed.Text != "starting agent\nnot json" ||
		len(parsed.Diagnostics) != 2 ||
		parsed.Diagnostics[0].Line != 1 ||
		parsed.Diagnostics[1].Line != 3 {
		t.Fatalf("parsed = %+v", parsed)
	}
	assertJSONField(t, parsed.Documents[0], "type", "finding")
	assertJSONField(t, parsed.Documents[1], "type", "summary")
}

func TestParseDelimitedJSONFallsBackWhenNoStructuredLines(t *testing.T) {
	t.Parallel()

	raw := []byte("starting\nstill running\n")
	parsed := Parse(raw, agents.OutputJSONL)
	if parsed.Structured ||
		parsed.Mode != agents.OutputText ||
		parsed.Text != string(raw) ||
		len(parsed.Documents) != 0 ||
		len(parsed.Diagnostics) != 2 {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseEmptyOutputFallsBackWithDiagnostic(t *testing.T) {
	t.Parallel()

	parsed := Parse([]byte(" \n\t"), agents.OutputJSON)
	if parsed.Structured ||
		parsed.Mode != agents.OutputText ||
		parsed.Text != " \n\t" ||
		len(parsed.Documents) != 0 ||
		len(parsed.Diagnostics) != 1 ||
		parsed.Diagnostics[0].Code != "empty_output" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseTextModePreservesRawOutput(t *testing.T) {
	t.Parallel()

	raw := []byte("plain review output\n- issue one\n")
	parsed := Parse(raw, agents.OutputText)
	if parsed.Structured ||
		parsed.Mode != agents.OutputText ||
		parsed.Text != string(raw) ||
		len(parsed.Documents) != 0 ||
		len(parsed.Diagnostics) != 0 {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseAutoPrefersJSONThenDelimitedJSONThenText(t *testing.T) {
	t.Parallel()

	jsonParsed := ParseAuto([]byte("{\"ok\":true}"))
	if !jsonParsed.Structured || jsonParsed.Mode != agents.OutputJSON || len(jsonParsed.Documents) != 1 {
		t.Fatalf("jsonParsed = %+v", jsonParsed)
	}

	delimitedParsed := ParseAuto([]byte("log\n{\"ok\":true}\n"))
	if !delimitedParsed.Structured ||
		delimitedParsed.Mode != agents.OutputJSONL ||
		len(delimitedParsed.Documents) != 1 ||
		delimitedParsed.Text != "log" {
		t.Fatalf("delimitedParsed = %+v", delimitedParsed)
	}

	textParsed := ParseAuto([]byte("not json"))
	if textParsed.Structured || textParsed.Mode != agents.OutputText || textParsed.Text != "not json" {
		t.Fatalf("textParsed = %+v", textParsed)
	}
}

func assertJSONField(t *testing.T, document json.RawMessage, key string, want string) {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("Unmarshal(document) error = %v", err)
	}
	if decoded[key] != want {
		t.Fatalf("decoded[%q] = %v, want %q", key, decoded[key], want)
	}
}
