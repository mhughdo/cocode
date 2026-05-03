package agents

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrJSONRPCStdioDisabled = errors.New("json-rpc stdio connections are disabled for the MVP")

type JSONRPCStdioDriver struct {
	Enabled bool
}

type JSONRPCStdioConnection struct {
	config ConnectionConfig
}

func (d JSONRPCStdioDriver) Open(ctx context.Context, config ConnectionConfig) (Connection, error) {
	ctx = commandContextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Kind != AdapterJSONRPCStdio && config.Kind != AdapterACPStdio {
		return nil, errors.New("json-rpc stdio driver requires jsonrpc_stdio or acp_stdio adapter kind")
	}
	if strings.TrimSpace(config.Command) == "" {
		return nil, errors.New("json-rpc stdio driver requires a command")
	}
	if !d.Enabled {
		return nil, ErrJSONRPCStdioDisabled
	}
	return nil, errors.New("json-rpc stdio driver is a future skeleton and has no runtime implementation")
}

func (c *JSONRPCStdioConnection) SendTask(ctx context.Context, task AgentTask) (<-chan AgentEvent, error) {
	ctx = commandContextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	events := make(chan AgentEvent, 1)
	events <- AgentEvent{
		Type:      EventFailed,
		RunID:     task.RunID,
		At:        time.Now().UTC(),
		Message:   "json-rpc stdio connection is disabled",
		ErrorCode: "unsupported",
		Error:     ErrJSONRPCStdioDisabled.Error(),
	}
	close(events)
	return events, nil
}

func (c *JSONRPCStdioConnection) Close(context.Context) error {
	return nil
}

var _ ConnectionDriver = JSONRPCStdioDriver{}
var _ Connection = (*JSONRPCStdioConnection)(nil)
