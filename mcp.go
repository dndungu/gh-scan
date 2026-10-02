package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolInput struct {
	OrgFilter       string `json:"org_filter,omitempty"`
	IncludePersonal *bool  `json:"include_personal,omitempty"`
}

func rawID(id json.RawMessage) interface{} {
	if len(id) == 0 {
		return nil
	}
	return id
}

func RunMCP(ctx context.Context, s *Scanner) error {
	reader := bufio.NewReader(os.Stdin)
	var mu sync.Mutex
	writer := func(v interface{}) {
		mu.Lock()
		defer mu.Unlock()
		data, _ := json.Marshal(v)
		fmt.Fprintln(os.Stdout, string(data))
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if line == "\n" || line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			writer(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      rawID(req.ID),
				Result: map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
					"serverInfo": map[string]interface{}{
						"name":    "gh-scan",
						"version": "1.0.0",
					},
				},
			})

		case "notifications/initialized":
			// no response needed

		case "tools/list":
			writer(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      rawID(req.ID),
				Result: map[string]interface{}{
					"tools": []interface{}{
						map[string]interface{}{
							"name":        "scan_repositories",
							"description": "Scan GitHub repositories; save READMEs under outputs/ and return a Markdown inventory with descriptions.",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"org_filter":       map[string]string{"type": "string", "description": "Optional comma-separated org list"},
									"include_personal": map[string]interface{}{"type": "boolean", "description": "Include personal account repos (default true)"},
								},
							},
						},
					},
				},
			})

		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			json.Unmarshal(req.Params, &params)

			var input toolInput
			json.Unmarshal(params.Arguments, &input)
			includePersonal := true
			if input.IncludePersonal != nil {
				includePersonal = *input.IncludePersonal
			}

			docs, err := s.Collect(ctx, input.OrgFilter, includePersonal)
			if err != nil {
				writer(jsonRPCResponse{
					JSONRPC: "2.0", ID: rawID(req.ID),
					Error: &rpcError{Code: -32000, Message: err.Error()},
				})
				continue
			}
			if err := WriteReadmes(docs, "outputs"); err != nil {
				writer(jsonRPCResponse{
					JSONRPC: "2.0", ID: rawID(req.ID),
					Error: &rpcError{Code: -32000, Message: err.Error()},
				})
				continue
			}
			md := RenderMarkdown(docs)
			if len(md) > 900_000 {
				md = md[:900_000] + "\n\n... [truncated]"
			}
			writer(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      rawID(req.ID),
				Result: map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": md},
					},
				},
			})
		case "ping":
			writer(jsonRPCResponse{JSONRPC: "2.0", ID: rawID(req.ID), Result: map[string]interface{}{}})

		default:
			writer(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      rawID(req.ID),
				Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
			})
		}
	}
}
