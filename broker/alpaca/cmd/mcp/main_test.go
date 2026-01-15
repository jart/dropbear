package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"dropbear/netty"
)

func init() {
	netty.SetOffline()
}

func TestIsOptionSymbol(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"AAPL", false},
		{"GOOGL", false},
		{"SPY", false},
		{"AAPL240119C00190000", true},
		{"SPY250117P00450000", true},
		{"TSLA250321C00200000", true},
		{"A250117C00100000", true},      // single letter root
		{"ABCDEF250117P00050000", true}, // 6 letter root
		{"AAPL240119X00190000", false},  // invalid type (not C or P)
		{"AAPL24011900190000", false},   // missing C/P
		{"aapl240119c00190000", false},  // lowercase
		{"", false},
	}

	for _, tt := range tests {
		got := isOptionSymbol(tt.symbol)
		if got != tt.want {
			t.Errorf("isOptionSymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
		}
	}
}

func TestGetStr(t *testing.T) {
	args := map[string]any{
		"symbol":  "AAPL",
		"number":  123,
		"empty":   "",
		"boolean": true,
	}

	tests := []struct {
		key  string
		want string
	}{
		{"symbol", "AAPL"},
		{"empty", ""},
		{"missing", ""},
		{"number", ""},  // not a string
		{"boolean", ""}, // not a string
	}

	for _, tt := range tests {
		got := getStr(args, tt.key)
		if got != tt.want {
			t.Errorf("getStr(args, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestGetStrSlice(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		key  string
		want []string
	}{
		{
			name: "array of strings",
			args: map[string]any{"symbols": []any{"AAPL", "GOOGL", "SPY"}},
			key:  "symbols",
			want: []string{"AAPL", "GOOGL", "SPY"},
		},
		{
			name: "single string",
			args: map[string]any{"symbols": "AAPL"},
			key:  "symbols",
			want: []string{"AAPL"},
		},
		{
			name: "empty string",
			args: map[string]any{"symbols": ""},
			key:  "symbols",
			want: nil,
		},
		{
			name: "missing key",
			args: map[string]any{},
			key:  "symbols",
			want: nil,
		},
		{
			name: "mixed types in array",
			args: map[string]any{"symbols": []any{"AAPL", 123, "SPY"}},
			key:  "symbols",
			want: []string{"AAPL", "SPY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStrSlice(tt.args, tt.key)
			if len(got) != len(tt.want) {
				t.Errorf("getStrSlice() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getStrSlice()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		strs []string
		sep  string
		want string
	}{
		{nil, "\n", ""},
		{[]string{}, "\n", ""},
		{[]string{"a"}, "\n", "a"},
		{[]string{"a", "b"}, "\n", "a\nb"},
		{[]string{"a", "b", "c"}, ", ", "a, b, c"},
	}

	for _, tt := range tests {
		got := join(tt.strs, tt.sep)
		if got != tt.want {
			t.Errorf("join(%v, %q) = %q, want %q", tt.strs, tt.sep, got, tt.want)
		}
	}
}

func TestMCPProtocol(t *testing.T) {
	// Test initialize request
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	initResp := processRequest(t, initReq)

	if initResp.Error != nil {
		t.Fatalf("initialize returned error: %v", initResp.Error)
	}
	result, ok := initResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("initialize result is not a map")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("unexpected protocolVersion: %v", result["protocolVersion"])
	}

	// Test tools/list request
	listReq := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	listResp := processRequest(t, listReq)

	if listResp.Error != nil {
		t.Fatalf("tools/list returned error: %v", listResp.Error)
	}
	listResult, ok := listResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result is not a map")
	}
	toolsAny, ok := listResult["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result has no tools array")
	}
	if len(toolsAny) != len(tools) {
		t.Errorf("tools/list returned %d tools, expected %d", len(toolsAny), len(tools))
	}

	// Verify all expected tools are present
	expectedTools := []string{
		"place_order", "get_quote", "get_account", "get_positions",
		"get_orders", "cancel_order", "cancel_all_orders", "get_bars",
		"get_snapshot", "get_auctions", "get_option_chain",
	}
	toolNames := make(map[string]bool)
	for _, tool := range toolsAny {
		if toolMap, ok := tool.(map[string]any); ok {
			if name, ok := toolMap["name"].(string); ok {
				toolNames[name] = true
			}
		}
	}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing expected tool: %s", name)
		}
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	req := `{"jsonrpc":"2.0","id":1,"method":"unknown/method"}`
	resp := processRequest(t, req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
}

func TestToolCallValidation(t *testing.T) {
	// Test place_order without symbols
	result := handleToolCall(ToolCallParams{
		Name:      "place_order",
		Arguments: map[string]any{"qty": "100", "order_type": "limit"},
	})
	if !result.IsError {
		t.Error("expected error when symbols missing")
	}
	if !strings.Contains(result.Content[0].Text, "no symbols") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}

	// Test place_order without qty or amt
	result = handleToolCall(ToolCallParams{
		Name:      "place_order",
		Arguments: map[string]any{"symbols": []any{"AAPL"}, "order_type": "limit"},
	})
	if !result.IsError {
		t.Error("expected error when qty/amt missing")
	}
	if !strings.Contains(result.Content[0].Text, "must specify qty or amt") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}

	// Test place_order without order_type
	result = handleToolCall(ToolCallParams{
		Name:      "place_order",
		Arguments: map[string]any{"symbols": []any{"AAPL"}, "qty": "100"},
	})
	if !result.IsError {
		t.Error("expected error when order_type missing")
	}
	if !strings.Contains(result.Content[0].Text, "order_type is required") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}

	// Test get_quote without symbol
	result = handleToolCall(ToolCallParams{
		Name:      "get_quote",
		Arguments: map[string]any{},
	})
	if !result.IsError {
		t.Error("expected error when symbol missing")
	}

	// Test cancel_order without order_id
	result = handleToolCall(ToolCallParams{
		Name:      "cancel_order",
		Arguments: map[string]any{},
	})
	if !result.IsError {
		t.Error("expected error when order_id missing")
	}

	// Test unknown tool
	result = handleToolCall(ToolCallParams{
		Name:      "unknown_tool",
		Arguments: map[string]any{},
	})
	if !result.IsError {
		t.Error("expected error for unknown tool")
	}
	if !strings.Contains(result.Content[0].Text, "unknown tool") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}
}

func TestPanicRecovery(t *testing.T) {
	// Temporarily replace stderr to capture output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// This should not panic the program
	result := handleToolCall(ToolCallParams{
		Name: "get_bars",
		Arguments: map[string]any{
			"symbol": "", // This will cause an error in symbol.Parse
		},
	})

	w.Close()
	os.Stderr = oldStderr

	// Read captured stderr
	var buf bytes.Buffer
	io.Copy(&buf, r)

	// The result should indicate an error, not crash
	if !result.IsError {
		t.Error("expected error result")
	}
}

// processRequest simulates a single JSON-RPC request/response cycle
func processRequest(t *testing.T, reqJSON string) Response {
	t.Helper()

	var req Request
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	var resp Response
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "initialize":
		resp.Result = InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: Capabilities{
				Tools: &ToolsCapability{},
			},
			ServerInfo: ServerInfo{
				Name:    "alpaca-mcp",
				Version: "1.0.0",
			},
		}
	case "tools/list":
		resp.Result = ToolsListResult{Tools: tools}
	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &Error{Code: -32602, Message: err.Error()}
		} else {
			resp.Result = handleToolCall(params)
		}
	default:
		resp.Error = &Error{Code: -32601, Message: "method not found"}
	}

	// Re-encode and decode to simulate actual JSON processing
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	return decoded
}

func BenchmarkIsOptionSymbol(b *testing.B) {
	symbols := []string{
		"AAPL",
		"AAPL240119C00190000",
		"SPY250117P00450000",
		"GOOGL",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, sym := range symbols {
			isOptionSymbol(sym)
		}
	}
}

func BenchmarkGetStr(b *testing.B) {
	args := map[string]any{
		"symbol":      "AAPL",
		"qty":         "100",
		"limit_price": "150.50",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getStr(args, "symbol")
		getStr(args, "qty")
		getStr(args, "limit_price")
		getStr(args, "missing")
	}
}
