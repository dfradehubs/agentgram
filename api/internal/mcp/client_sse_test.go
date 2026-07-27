package mcp

import (
	"strings"
	"testing"
)

// n8n's built-in MCP server answers POSTs with text/event-stream, so tools/list
// arrives as a single SSE event whose data line holds the whole JSON-RPC payload
// (~13KB for a real instance).
func TestExtractJSONFromSSE(t *testing.T) {
	bigPayload := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"a","description":"` +
		strings.Repeat("x", 13_000) + `"}]}}`
	hugePayload := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"a","description":"` +
		strings.Repeat("x", 200_000) + `"}]}}`

	tests := []struct {
		name   string
		stream string
		want   string
	}{
		{
			name:   "small payload",
			stream: "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n",
			want:   `{"jsonrpc":"2.0","id":1,"result":{}}`,
		},
		{
			name:   "payload larger than the default scanner buffer",
			stream: "event: message\ndata: " + bigPayload + "\n\n",
			want:   bigPayload,
		},
		{
			// n8n gzips the response, so a 13KB body on the wire can be a
			// several-hundred-KB tools/list once decompressed. This is the case
			// that used to surface as "unexpected end of JSON input".
			name:   "payload larger than the 64KiB scanner token limit",
			stream: "event: message\ndata: " + hugePayload + "\n\n",
			want:   hugePayload,
		},
		{
			name:   "no space after data:",
			stream: "event: message\ndata:{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n",
			want:   `{"jsonrpc":"2.0","id":1,"result":{}}`,
		},
		{
			name:   "id line before the data line",
			stream: "id: 1\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n",
			want:   `{"jsonrpc":"2.0","id":1,"result":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSONFromSSE(strings.NewReader(tt.stream))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %d bytes, want %d bytes\ngot:  %.80s\nwant: %.80s",
					len(got), len(tt.want), got, tt.want)
			}
		})
	}
}
