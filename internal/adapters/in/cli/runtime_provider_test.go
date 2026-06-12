package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/pkg/plugin/protocol"
)

func TestProviderRuntimeCommandRegistersHidden(t *testing.T) {
	cmd := NewProviderRuntimeCommand(func(context.Context, string, protocol.Request) protocol.Response {
		return protocol.Response{}
	})
	require.NotNil(t, cmd)
	assert.Equal(t, "__provider", cmd.Use)
	assert.True(t, cmd.Hidden)
}

func TestProviderRuntimeCommandRejectsNilHandler(t *testing.T) {
	cmd := NewProviderRuntimeCommand(nil)
	cmd.SetArgs([]string{"codex"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler is nil")
}

func TestProviderRuntimeDispatchesValidRequests(t *testing.T) {
	input := `{"id":"init-1","method":"initialize","params":{"protocol":"ero.plugin.v1","contribution_id":"codex"}}
{"id":"detect-1","method":"detect_context","params":{}}
`
	var stdin bytes.Buffer
	stdin.WriteString(input)
	var stdout bytes.Buffer

	var seen []string
	handler := func(_ context.Context, providerID string, req protocol.Request) protocol.Response {
		seen = append(seen, providerID+":"+req.Method)
		return protocol.Response{Result: map[string]string{"ok": req.Method}}
	}

	err := runProviderRuntime(context.Background(), "codex", &stdin, &stdout, handler)
	require.NoError(t, err)
	assert.Equal(t, []string{"codex:initialize", "codex:detect_context"}, seen)

	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)

	var first protocol.Response
	require.NoError(t, json.Unmarshal(lines[0], &first))
	assert.Equal(t, "init-1", first.ID)
	assert.Nil(t, first.Error)
}

func TestProviderRuntimeSkipsEmptyLines(t *testing.T) {
	input := "\n\n{\"id\":\"1\",\"method\":\"detect_context\",\"params\":{}}\n\n"
	var stdin bytes.Buffer
	stdin.WriteString(input)
	var stdout bytes.Buffer

	err := runProviderRuntime(context.Background(), "codex", &stdin, &stdout, func(_ context.Context, _ string, _ protocol.Request) protocol.Response {
		return protocol.Response{Result: map[string]bool{"ok": true}}
	})
	require.NoError(t, err)

	var resp protocol.Response
	err = json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &resp)
	require.NoError(t, err)
	assert.Equal(t, "1", resp.ID)
}

func TestProviderRuntimeMalformedLineReturnsInvalidRequest(t *testing.T) {
	// A malformed JSON line must produce an error response, not a silent skip.
	input := `{"id":"ok","method":"detect_context","params":{}}
not-json-at-all
{"id":"also-ok","method":"detect_context","params":{}}`
	var stdin bytes.Buffer
	stdin.WriteString(input)
	var stdout bytes.Buffer

	err := runProviderRuntime(context.Background(), "codex", &stdin, &stdout, func(_ context.Context, _ string, _ protocol.Request) protocol.Response {
		return protocol.Response{Result: map[string]bool{"ok": true}}
	})
	require.NoError(t, err)

	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	require.Len(t, lines, 3, "expected 3 responses: 1 ok, 1 error, 1 ok")

	// First line: valid request.
	var first protocol.Response
	require.NoError(t, json.Unmarshal(lines[0], &first))
	assert.Equal(t, "ok", first.ID)
	assert.Nil(t, first.Error)

	// Second line: malformed — should be an error with empty ID.
	var second protocol.Response
	require.NoError(t, json.Unmarshal(lines[1], &second))
	assert.Equal(t, "", second.ID, "malformed line yields empty ID")
	require.NotNil(t, second.Error)
	assert.Equal(t, protocol.ErrorInvalidRequest, second.Error.Code)
	assert.Contains(t, second.Error.Message, "malformed")

	// Third line: valid request after the error.
	var third protocol.Response
	require.NoError(t, json.Unmarshal(lines[2], &third))
	assert.Equal(t, "also-ok", third.ID)
	assert.Nil(t, third.Error)
}
