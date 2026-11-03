package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
)

const metaProtocolVersion = "io.modelcontextprotocol/protocolVersion"

// newProtocol is the first version that requires the standard Mcp-Method /
// Mcp-Name request headers on streamable HTTP.
const newProtocol = "2026-07-28"

type rpcHead struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

func (h rpcHead) isRequest() bool { return h.Method != "" && len(h.ID) > 0 && string(h.ID) != "null" }

// Attach proxies newline-delimited JSON-RPC between in/out (a stdio MCP
// client) and the streamable-HTTP endpoint of a running server. Every request
// line becomes one POST; every JSON-RPC message in the response is written to
// out as one line. It returns when in reaches EOF or ctx ends.
func Attach(ctx context.Context, endpoint string, in io.Reader, out io.Writer) error {
	client := &http.Client{}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 64<<20)
	var mu sync.Mutex
	emit := func(msg []byte) {
		msg = bytes.TrimSpace(msg)
		if len(msg) == 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		out.Write(append(append([]byte{}, msg...), '\n'))
	}
	negotiated := ""
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var head rpcHead
		_ = json.Unmarshal(line, &head) // batches and responses keep a zero head

		version := negotiated
		var p struct {
			Name string         `json:"name"`
			URI  string         `json:"uri"`
			Meta map[string]any `json:"_meta"`
		}
		_ = json.Unmarshal(head.Params, &p)
		if v, ok := p.Meta[metaProtocolVersion].(string); ok && v != "" {
			version = v
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(line))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if version != "" && head.Method != "initialize" {
			req.Header.Set("Mcp-Protocol-Version", version)
			if version >= newProtocol && head.Method != "" {
				req.Header.Set("Mcp-Method", head.Method)
				switch head.Method {
				case "tools/call", "prompts/get":
					req.Header.Set("Mcp-Name", p.Name)
				case "resources/read":
					req.Header.Set("Mcp-Name", p.URI)
				}
			}
		}
		res, err := client.Do(req)
		if err != nil {
			if head.isRequest() {
				emit(rpcError(head.ID, -32000, "AgentCanvas server unreachable: "+err.Error()))
			}
			continue
		}
		msgs, body := readMessages(res)
		res.Body.Close()
		if len(msgs) == 0 && res.StatusCode >= 400 && head.isRequest() {
			emit(rpcError(head.ID, -32000, fmt.Sprintf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))))
			continue
		}
		for _, m := range msgs {
			if head.Method == "initialize" {
				var r rpcHead
				if json.Unmarshal(m, &r) == nil && len(r.Result) > 0 {
					var ir struct {
						ProtocolVersion string `json:"protocolVersion"`
					}
					if json.Unmarshal(r.Result, &ir) == nil && ir.ProtocolVersion != "" {
						negotiated = ir.ProtocolVersion
					}
				}
			}
			emit(m)
		}
	}
	return sc.Err()
}

func rpcError(id json.RawMessage, code int, msg string) []byte {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
	return b
}

// readMessages extracts JSON-RPC messages from a JSON or SSE response body.
func readMessages(res *http.Response) ([][]byte, []byte) {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	ct, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	var msgs [][]byte
	switch ct {
	case "application/json":
		trimmed := bytes.TrimSpace(body)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			var arr []json.RawMessage
			if json.Unmarshal(trimmed, &arr) == nil {
				for _, m := range arr {
					msgs = append(msgs, compact(m))
				}
			}
		} else if len(trimmed) > 0 && json.Valid(trimmed) {
			msgs = append(msgs, compact(trimmed))
		}
	case "text/event-stream":
		var data []string
		flush := func() {
			if len(data) > 0 {
				d := []byte(strings.Join(data, "\n"))
				if json.Valid(d) {
					msgs = append(msgs, compact(d))
				}
				data = nil
			}
		}
		for _, l := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
			switch {
			case l == "":
				flush()
			case strings.HasPrefix(l, "data:"):
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(l, "data:"), " "))
			}
		}
		flush()
	}
	return msgs, body
}

func compact(b []byte) []byte {
	var buf bytes.Buffer
	if json.Compact(&buf, b) != nil {
		return b
	}
	return buf.Bytes()
}
