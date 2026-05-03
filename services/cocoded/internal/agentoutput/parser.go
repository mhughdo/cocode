package agentoutput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hughdo/cocode/services/cocoded/internal/agents"
)

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

type ParsedOutput struct {
	Mode        agents.OutputMode `json:"mode"`
	Structured  bool              `json:"structured"`
	Documents   []json.RawMessage `json:"documents,omitempty"`
	Text        string            `json:"text,omitempty"`
	Diagnostics []Diagnostic      `json:"diagnostics,omitempty"`
}

func Parse(content []byte, mode agents.OutputMode) ParsedOutput {
	if mode == "" {
		mode = agents.OutputText
	}
	switch mode {
	case agents.OutputJSON:
		return parseJSON(content)
	case agents.OutputJSONL, agents.OutputNDJSON:
		return parseDelimitedJSON(content, mode)
	default:
		return textOutput(content, nil)
	}
}

func ParseAuto(content []byte) ParsedOutput {
	parsed := parseJSON(content)
	if parsed.Structured {
		return parsed
	}
	delimited := parseDelimitedJSON(content, agents.OutputJSONL)
	if delimited.Structured {
		return delimited
	}
	return textOutput(content, parsed.Diagnostics)
}

func parseJSON(content []byte) ParsedOutput {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return textOutput(content, []Diagnostic{{Code: "empty_output", Message: "output is empty"}})
	}
	if !json.Valid(trimmed) {
		return textOutput(content, []Diagnostic{{Code: "invalid_json", Message: "output is not valid JSON"}})
	}
	return ParsedOutput{
		Mode:       agents.OutputJSON,
		Structured: true,
		Documents:  []json.RawMessage{append(json.RawMessage(nil), trimmed...)},
		Text:       string(content),
	}
}

func parseDelimitedJSON(content []byte, mode agents.OutputMode) ParsedOutput {
	lines := bytes.Split(content, []byte{'\n'})
	documents := make([]json.RawMessage, 0, len(lines))
	diagnostics := []Diagnostic{}
	textLines := []string{}
	for index, line := range lines {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if json.Valid(trimmed) {
			documents = append(documents, append(json.RawMessage(nil), trimmed...))
			continue
		}
		textLines = append(textLines, string(line))
		diagnostics = append(diagnostics, Diagnostic{
			Code:    "invalid_json_line",
			Message: fmt.Sprintf("line %d is not valid JSON", index+1),
			Line:    index + 1,
		})
	}
	if len(documents) == 0 {
		if len(diagnostics) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Code: "empty_output", Message: "output is empty"})
		}
		return textOutput(content, diagnostics)
	}
	return ParsedOutput{
		Mode:        mode,
		Structured:  true,
		Documents:   documents,
		Text:        strings.Join(textLines, "\n"),
		Diagnostics: diagnostics,
	}
}

func textOutput(content []byte, diagnostics []Diagnostic) ParsedOutput {
	return ParsedOutput{
		Mode:        agents.OutputText,
		Text:        string(content),
		Diagnostics: diagnostics,
	}
}
