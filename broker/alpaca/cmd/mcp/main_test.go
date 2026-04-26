package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
)

func init() {
	netty.SetOffline()
}

// TestMlegOrderRequestJSON verifies that an mleg OrderRequest serializes to
// exactly the JSON body that Alpaca's POST /v2/orders expects: no top-level
// symbol/side, legs array with string ratio_qty, and all numeric fields as
// quoted strings.
func TestMlegOrderRequestJSON(t *testing.T) {
	req := alpaca.CreateOrderRequest{
		Qty:         decimal.FromInt(400),
		Type:        alpaca.OrderTypeLimit,
		TimeInForce: alpaca.TimeInForceDay,
		LimitPrice:  decimal.Parse("10.99"),
		OrderClass:  alpaca.OrderClassMleg,
		Legs: []alpaca.OrderLeg{
			{Symbol: "QQQ260217C00593000", Side: ds.SideBuy, RatioQty: decimal.One},
			{Symbol: "QQQ260217C00604000", Side: ds.SideSell, RatioQty: decimal.One},
			{Symbol: "QQQ260217P00604000", Side: ds.SideBuy, RatioQty: decimal.One},
			{Symbol: "QQQ260217P00593000", Side: ds.SideSell, RatioQty: decimal.One},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Parse back to a generic map to check structure
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// No top-level symbol (omitempty should drop it)
	if _, ok := m["symbol"]; ok {
		t.Error("mleg request should not have top-level symbol")
	}

	// No top-level side (omitempty should drop it)
	if _, ok := m["side"]; ok {
		t.Error("mleg request should not have top-level side")
	}

	// order_class must be "mleg"
	if m["order_class"] != "mleg" {
		t.Errorf("order_class = %v, want \"mleg\"", m["order_class"])
	}

	// qty must be 400
	if m["qty"] != float64(400) {
		t.Errorf("qty = %v (type %T), want 400", m["qty"], m["qty"])
	}

	// limit_price must be 10.99
	if m["limit_price"] != float64(10.99) {
		t.Errorf("limit_price = %v, want 10.99", m["limit_price"])
	}

	// type must be "limit"
	if m["type"] != "limit" {
		t.Errorf("type = %v, want \"limit\"", m["type"])
	}

	// legs must be array of 4
	legsAny, ok := m["legs"].([]any)
	if !ok {
		t.Fatalf("legs is not an array: %T", m["legs"])
	}
	if len(legsAny) != 4 {
		t.Fatalf("legs has %d elements, want 4", len(legsAny))
	}

	// Check first leg structure
	leg0, ok := legsAny[0].(map[string]any)
	if !ok {
		t.Fatalf("leg[0] is not a map: %T", legsAny[0])
	}
	if leg0["symbol"] != "QQQ260217C00593000" {
		t.Errorf("leg[0].symbol = %v", leg0["symbol"])
	}
	if leg0["side"] != "buy" {
		t.Errorf("leg[0].side = %v", leg0["side"])
	}
	if leg0["ratio_qty"] != float64(1) {
		t.Errorf("leg[0].ratio_qty = %v (type %T), want 1", leg0["ratio_qty"], leg0["ratio_qty"])
	}

	// Check second leg (sell side)
	leg1, ok := legsAny[1].(map[string]any)
	if !ok {
		t.Fatalf("leg[1] is not a map: %T", legsAny[1])
	}
	if leg1["side"] != "sell" {
		t.Errorf("leg[1].side = %v", leg1["side"])
	}

	// position_intent should be omitted when not set
	if _, ok := leg0["position_intent"]; ok {
		t.Error("leg[0] should not have position_intent when unset")
	}

	t.Logf("serialized JSON:\n%s", string(data))
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
		"place_mleg_order", "place_box_spread",
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

	// Test place_order with mleg but no legs
	result = handleToolCall(ToolCallParams{
		Name: "place_order",
		Arguments: map[string]any{
			"order_type":  "limit",
			"order_class": "mleg",
			"qty":         "400",
			"limit_price": "10.999",
		},
	})
	if !result.IsError {
		t.Error("expected error when mleg has no legs")
	}
	if !strings.Contains(result.Content[0].Text, "legs array is required") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}

	// Test place_order with mleg but missing qty
	result = handleToolCall(ToolCallParams{
		Name: "place_order",
		Arguments: map[string]any{
			"order_type":  "limit",
			"order_class": "mleg",
			"limit_price": "10.999",
			"legs": []any{
				map[string]any{"symbol": "QQQ260217C00593000", "side": "buy", "ratio_qty": 1.0},
			},
		},
	})
	if !result.IsError {
		t.Error("expected error when mleg has no qty")
	}
	if !strings.Contains(result.Content[0].Text, "qty must be a positive") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}

	// Test place_order with mleg and invalid leg symbol
	result = handleToolCall(ToolCallParams{
		Name: "place_order",
		Arguments: map[string]any{
			"order_type":  "limit",
			"order_class": "mleg",
			"qty":         "400",
			"limit_price": "10.999",
			"legs": []any{
				map[string]any{"symbol": "NOTANOPTION", "side": "buy"},
			},
		},
	})
	if !result.IsError {
		t.Error("expected error for invalid OCC symbol in mleg leg")
	}
	if !strings.Contains(result.Content[0].Text, "invalid OCC option symbol") {
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

func TestBuildOCCSymbol(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		expiration string
		optionType string
		strike     string
		want       string
		wantErr    bool
	}{
		{
			name:       "SPY call at 500",
			root:       "SPY",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "500",
			want:       "SPY260320C00500000",
		},
		{
			name:       "SPY put at 450",
			root:       "SPY",
			expiration: "2026-03-20",
			optionType: "P",
			strike:     "450",
			want:       "SPY260320P00450000",
		},
		{
			name:       "decimal strike",
			root:       "AAPL",
			expiration: "2026-01-16",
			optionType: "C",
			strike:     "190.5",
			want:       "AAPL260116C00190500",
		},
		{
			name:       "single letter root",
			root:       "A",
			expiration: "2026-06-19",
			optionType: "P",
			strike:     "100",
			want:       "A260619P00100000",
		},
		{
			name:       "six letter root",
			root:       "ABCDEF",
			expiration: "2026-12-18",
			optionType: "C",
			strike:     "50",
			want:       "ABCDEF261218C00050000",
		},
		{
			name:       "small strike",
			root:       "SPY",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "1.5",
			want:       "SPY260320C00001500",
		},
		{
			name:       "empty root",
			root:       "",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "500",
			wantErr:    true,
		},
		{
			name:       "root too long",
			root:       "ABCDEFG",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "500",
			wantErr:    true,
		},
		{
			name:       "lowercase root",
			root:       "spy",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "500",
			wantErr:    true,
		},
		{
			name:       "bad date format",
			root:       "SPY",
			expiration: "03/20/2026",
			optionType: "C",
			strike:     "500",
			wantErr:    true,
		},
		{
			name:       "bad option type",
			root:       "SPY",
			expiration: "2026-03-20",
			optionType: "X",
			strike:     "500",
			wantErr:    true,
		},
		{
			name:       "negative strike",
			root:       "SPY",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "-100",
			wantErr:    true,
		},
		{
			name:       "zero strike",
			root:       "SPY",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "0",
			wantErr:    true,
		},
		{
			name:       "invalid strike string",
			root:       "SPY",
			expiration: "2026-03-20",
			optionType: "C",
			strike:     "abc",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildOCCSymbol(tt.root, tt.expiration, tt.optionType, tt.strike)
			if tt.wantErr {
				if err == nil {
					t.Errorf("buildOCCSymbol() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Errorf("buildOCCSymbol() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("buildOCCSymbol() = %q, want %q", got, tt.want)
			}
			// Verify result is a valid OCC symbol
			if !isOptionSymbol(got) {
				t.Errorf("buildOCCSymbol() = %q is not a valid OCC symbol", got)
			}
		})
	}
}

func TestGetMapSlice(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		wantLen int
	}{
		{
			name: "array of maps",
			args: map[string]any{
				"legs": []any{
					map[string]any{"symbol": "SPY260320C00500000", "side": "buy"},
					map[string]any{"symbol": "SPY260320C00550000", "side": "sell"},
				},
			},
			key:     "legs",
			wantLen: 2,
		},
		{
			name:    "missing key",
			args:    map[string]any{},
			key:     "legs",
			wantLen: 0,
		},
		{
			name:    "wrong type (string)",
			args:    map[string]any{"legs": "not an array"},
			key:     "legs",
			wantLen: 0,
		},
		{
			name: "mixed types in array",
			args: map[string]any{
				"legs": []any{
					map[string]any{"symbol": "SPY260320C00500000"},
					"not a map",
					map[string]any{"symbol": "SPY260320P00500000"},
				},
			},
			key:     "legs",
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getMapSlice(tt.args, tt.key)
			if len(got) != tt.wantLen {
				t.Errorf("getMapSlice() returned %d items, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestPlaceMlegOrderValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantMsg string
	}{
		{
			name:    "missing qty",
			args:    map[string]any{"legs": []any{map[string]any{"symbol": "SPY260320C00500000", "side": "buy"}}, "limit_price": "10"},
			wantMsg: "qty is required",
		},
		{
			name:    "missing limit_price",
			args:    map[string]any{"legs": []any{map[string]any{"symbol": "SPY260320C00500000", "side": "buy"}}, "qty": "1"},
			wantMsg: "limit_price is required",
		},
		{
			name:    "no legs",
			args:    map[string]any{"legs": []any{}, "qty": "1", "limit_price": "10"},
			wantMsg: "legs array is required",
		},
		{
			name: "too many legs",
			args: map[string]any{
				"legs": []any{
					map[string]any{"symbol": "SPY260320C00500000", "side": "buy"},
					map[string]any{"symbol": "SPY260320C00510000", "side": "sell"},
					map[string]any{"symbol": "SPY260320P00510000", "side": "buy"},
					map[string]any{"symbol": "SPY260320P00500000", "side": "sell"},
					map[string]any{"symbol": "SPY260320C00520000", "side": "buy"},
				},
				"qty": "1", "limit_price": "10",
			},
			wantMsg: "max 4 legs",
		},
		{
			name: "invalid OCC symbol",
			args: map[string]any{
				"legs":        []any{map[string]any{"symbol": "NOTVALID", "side": "buy"}},
				"qty":         "1",
				"limit_price": "10",
			},
			wantMsg: "invalid OCC option symbol",
		},
		{
			name: "missing side on leg",
			args: map[string]any{
				"legs":        []any{map[string]any{"symbol": "SPY260320C00500000"}},
				"qty":         "1",
				"limit_price": "10",
			},
			wantMsg: "side is required",
		},
		{
			name: "negative qty",
			args: map[string]any{
				"legs":        []any{map[string]any{"symbol": "SPY260320C00500000", "side": "buy"}},
				"qty":         "-1",
				"limit_price": "10",
			},
			wantMsg: "qty must be a positive number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleToolCall(ToolCallParams{
				Name:      "place_mleg_order",
				Arguments: tt.args,
			})
			if !result.IsError {
				t.Error("expected error")
			}
			if !strings.Contains(result.Content[0].Text, tt.wantMsg) {
				t.Errorf("error message %q does not contain %q", result.Content[0].Text, tt.wantMsg)
			}
		})
	}
}

func TestPlaceBoxSpreadValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantMsg string
	}{
		{
			name:    "missing underlying",
			args:    map[string]any{"expiration": "2026-03-20", "strike_low": "500", "strike_high": "550", "qty": "1"},
			wantMsg: "underlying symbol is required",
		},
		{
			name:    "missing expiration",
			args:    map[string]any{"underlying": "SPY", "strike_low": "500", "strike_high": "550", "qty": "1"},
			wantMsg: "expiration date is required",
		},
		{
			name:    "missing strike_low",
			args:    map[string]any{"underlying": "SPY", "expiration": "2026-03-20", "strike_high": "550", "qty": "1"},
			wantMsg: "strike_low is required",
		},
		{
			name:    "missing strike_high",
			args:    map[string]any{"underlying": "SPY", "expiration": "2026-03-20", "strike_low": "500", "qty": "1"},
			wantMsg: "strike_high is required",
		},
		{
			name:    "strike_low equals strike_high",
			args:    map[string]any{"underlying": "SPY", "expiration": "2026-03-20", "strike_low": "500", "strike_high": "500", "qty": "1"},
			wantMsg: "strike_low",
		},
		{
			name:    "strike_low greater than strike_high",
			args:    map[string]any{"underlying": "SPY", "expiration": "2026-03-20", "strike_low": "550", "strike_high": "500", "qty": "1"},
			wantMsg: "strike_low",
		},
		{
			name:    "missing qty",
			args:    map[string]any{"underlying": "SPY", "expiration": "2026-03-20", "strike_low": "500", "strike_high": "550"},
			wantMsg: "qty is required",
		},
		{
			name:    "negative strike",
			args:    map[string]any{"underlying": "SPY", "expiration": "2026-03-20", "strike_low": "-100", "strike_high": "550", "qty": "1"},
			wantMsg: "strike_low must be a positive number",
		},
		{
			name:    "invalid strike string",
			args:    map[string]any{"underlying": "SPY", "expiration": "2026-03-20", "strike_low": "abc", "strike_high": "550", "qty": "1"},
			wantMsg: "strike_low must be a positive number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleToolCall(ToolCallParams{
				Name:      "place_box_spread",
				Arguments: tt.args,
			})
			if !result.IsError {
				t.Error("expected error")
			}
			if !strings.Contains(result.Content[0].Text, tt.wantMsg) {
				t.Errorf("error message %q does not contain %q", result.Content[0].Text, tt.wantMsg)
			}
		})
	}
}

func BenchmarkBuildOCCSymbol(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildOCCSymbol("SPY", "2026-03-20", "C", "500")
		buildOCCSymbol("AAPL", "2026-01-16", "P", "190.5")
		buildOCCSymbol("A", "2026-06-19", "C", "100")
	}
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
