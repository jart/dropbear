package main

import (
	"bufio"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/symbol"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// isOptionSymbol returns true if the symbol looks like an OCC options symbol.
// OCC format: ROOT + YYMMDD + C/P + STRIKE (e.g., AAPL240119C00190000)
var optionSymbolRegex = regexp.MustCompile(`^[A-Z]{1,6}\d{6}[CP]\d{8}$`)
var occDateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func isOptionSymbol(symbol string) bool {
	return optionSymbolRegex.MatchString(symbol)
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema JSONSchema `json:"inputSchema"`
}

type JSONSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Enum        []string    `json:"enum,omitempty"`
	Items       *JSONSchema `json:"items,omitempty"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var tools = []Tool{
	{
		Name:        "place_order",
		Description: "Place a stock or option order via Alpaca. Use positive qty/amt to buy, negative to sell. Supports OCC option symbols (e.g., AAPL240119C00190000).",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"symbols": {
					Type:        "array",
					Description: "Stock symbols to trade (e.g., AAPL, GOOGL)",
				},
				"qty": {
					Type:        "string",
					Description: "Number of shares to buy (positive) or sell (negative). Mutually exclusive with amt.",
				},
				"amt": {
					Type:        "string",
					Description: "USD notional value to buy (positive) or sell (negative). Mutually exclusive with qty.",
				},
				"order_type": {
					Type:        "string",
					Description: "Order type: limit, stop (market when triggered), stop_limit (limit when triggered), trailing_stop",
					Enum:        []string{"limit", "stop", "stop_limit", "trailing_stop"},
				},
				"limit_price": {
					Type:        "string",
					Description: "Limit price for limit/stop_limit orders",
				},
				"greed": {
					Type:        "string",
					Description: "Basis points improvement over midpoint (negative for better fill likelihood)",
				},
				"time_in_force": {
					Type:        "string",
					Description: "Time in force: day, gtc, opg, cls, fok, ioc",
					Enum:        []string{"day", "gtc", "opg", "cls", "fok", "ioc"},
				},
				"algorithm": {
					Type:        "string",
					Description: "Execution algorithm: none, twap, vwap",
					Enum:        []string{"none", "twap", "vwap"},
				},
				"duration": {
					Type:        "string",
					Description: "Duration for TWAP/VWAP orders (e.g., 1h30m)",
				},
				"participation": {
					Type:        "string",
					Description: "Max volume participation rate for TWAP/VWAP (default 0.15)",
				},
				"extended_hours": {
					Type:        "string",
					Description: "Participate in extended hours trading (true/false)",
				},
				"stop_price": {
					Type:        "string",
					Description: "Trigger price for stop/stop_limit orders, or stop loss price for bracket orders",
				},
				"trail_price": {
					Type:        "string",
					Description: "Dollar amount for trailing stop to trail behind market price",
				},
				"trail_percent": {
					Type:        "string",
					Description: "Percentage for trailing stop to trail behind market price (e.g., 1.5 for 1.5%)",
				},
				"bracket_stop_price": {
					Type:        "string",
					Description: "Stop loss price for bracket orders",
				},
				"bracket_stop_limit_price": {
					Type:        "string",
					Description: "Stop limit price for bracket orders (makes stop loss a stop-limit)",
				},
				"take_profit_price": {
					Type:        "string",
					Description: "Take profit limit price for bracket orders",
				},
				"order_class": {
					Type:        "string",
					Description: "Order class: simple, bracket, oco, oto, mleg (multi-leg options)",
					Enum:        []string{"simple", "bracket", "oco", "oto", "mleg"},
				},
				"legs": {
					Type:        "array",
					Description: "For mleg orders: array of leg objects (max 4). When using mleg, symbols/qty sign/amt are ignored; set qty to the positive contract count.",
					Items: &JSONSchema{
						Type: "object",
						Properties: map[string]Property{
							"symbol": {
								Type:        "string",
								Description: "OCC option symbol (e.g., QQQ260217C00593000)",
							},
							"side": {
								Type:        "string",
								Description: "buy or sell",
								Enum:        []string{"buy", "sell"},
							},
							"ratio_qty": {
								Type:        "integer",
								Description: "Ratio quantity for this leg (e.g., 1)",
							},
							"position_intent": {
								Type:        "string",
								Description: "Position intent for this leg",
								Enum:        []string{"buy_to_open", "buy_to_close", "sell_to_open", "sell_to_close"},
							},
						},
						Required: []string{"symbol", "side", "ratio_qty"},
					},
				},
			},
			Required: []string{"order_type"},
		},
	},
	{
		Name:        "get_quote",
		Description: "Get current bid/ask quote for a stock or option. Options also include IV and greeks.",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"symbol": {
					Type:        "string",
					Description: "Stock symbol (e.g., AAPL) or option symbol (e.g., AAPL240119C00190000)",
				},
			},
			Required: []string{"symbol"},
		},
	},
	{
		Name:        "get_account",
		Description: "Get Alpaca account info including cash, buying power, equity, margin, and day trade count",
		InputSchema: JSONSchema{
			Type:       "object",
			Properties: map[string]Property{},
		},
	},
	{
		Name:        "get_positions",
		Description: "Get all current positions in the Alpaca account",
		InputSchema: JSONSchema{
			Type:       "object",
			Properties: map[string]Property{},
		},
	},
	{
		Name:        "get_orders",
		Description: "Get orders in the Alpaca account with optional filtering",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"status": {
					Type:        "string",
					Description: "Order status filter: open, closed, or all (default: open)",
					Enum:        []string{"open", "closed", "all"},
				},
				"limit": {
					Type:        "string",
					Description: "Maximum number of orders to return (default 50, max 500)",
				},
				"symbols": {
					Type:        "string",
					Description: "Comma-separated list of symbols to filter by",
				},
				"direction": {
					Type:        "string",
					Description: "Sort direction: asc or desc (default: desc)",
					Enum:        []string{"asc", "desc"},
				},
			},
		},
	},
	{
		Name:        "cancel_order",
		Description: "Cancel an order by ID",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"order_id": {
					Type:        "string",
					Description: "The order ID to cancel",
				},
			},
			Required: []string{"order_id"},
		},
	},
	{
		Name:        "cancel_all_orders",
		Description: "Cancel all open orders",
		InputSchema: JSONSchema{
			Type:       "object",
			Properties: map[string]Property{},
		},
	},
	{
		Name:        "get_bars",
		Description: "Get historical OHLCV bars for a stock symbol",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"symbol": {
					Type:        "string",
					Description: "Stock symbol (e.g., AAPL)",
				},
				"timeframe": {
					Type:        "string",
					Description: "Bar timeframe: 1m, 5m, 15m, 30m, 1h, 4h, 1d, 1w, 1M",
					Enum:        []string{"1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w", "1M"},
				},
				"start": {
					Type:        "string",
					Description: "Start time in RFC3339 format (e.g., 2024-01-01T00:00:00Z)",
				},
				"end": {
					Type:        "string",
					Description: "End time in RFC3339 format (e.g., 2024-01-02T00:00:00Z)",
				},
				"limit": {
					Type:        "string",
					Description: "Maximum number of bars to return (default 100, max 10000)",
				},
				"feed": {
					Type:        "string",
					Description: "Data feed: sip (all exchanges), iex, otc",
					Enum:        []string{"sip", "iex", "otc"},
				},
				"adjustment": {
					Type:        "string",
					Description: "Price adjustment: raw, split, dividend, all",
					Enum:        []string{"raw", "split", "dividend", "all"},
				},
			},
			Required: []string{"symbol"},
		},
	},
	{
		Name:        "get_snapshot",
		Description: "Get a comprehensive market data snapshot. For stocks: latest trade, quote, minute/daily/prev bars. For options: latest trade, quote, IV, and greeks.",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"symbol": {
					Type:        "string",
					Description: "Stock symbol (e.g., AAPL) or option symbol (e.g., AAPL240119C00190000)",
				},
			},
			Required: []string{"symbol"},
		},
	},
	{
		Name:        "get_auctions",
		Description: "Get historical opening and closing auction data for a stock symbol",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"symbol": {
					Type:        "string",
					Description: "Stock symbol (e.g., AAPL)",
				},
				"start": {
					Type:        "string",
					Description: "Start time in RFC3339 format (e.g., 2024-01-01T00:00:00Z)",
				},
				"end": {
					Type:        "string",
					Description: "End time in RFC3339 format (e.g., 2024-01-02T00:00:00Z)",
				},
				"limit": {
					Type:        "string",
					Description: "Maximum number of auction days to return (default 100, max 10000)",
				},
				"feed": {
					Type:        "string",
					Description: "Data feed: sip (all exchanges), iex, otc",
					Enum:        []string{"sip", "iex", "otc"},
				},
			},
			Required: []string{"symbol"},
		},
	},
	{
		Name:        "place_mleg_order",
		Description: "Place a multi-leg (mleg) option order. Each leg specifies an OCC option symbol, side, ratio quantity, and optional position intent. Max 4 legs.",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"legs": {
					Type:        "array",
					Description: "Array of leg objects (max 4).",
					Items: &JSONSchema{
						Type: "object",
						Properties: map[string]Property{
							"symbol": {
								Type:        "string",
								Description: "OCC option symbol (e.g., SPY260320C00500000)",
							},
							"side": {
								Type:        "string",
								Description: "buy or sell",
								Enum:        []string{"buy", "sell"},
							},
							"ratio_qty": {
								Type:        "integer",
								Description: "Ratio quantity for this leg (e.g., 1)",
							},
							"position_intent": {
								Type:        "string",
								Description: "Position intent for this leg",
								Enum:        []string{"buy_to_open", "buy_to_close", "sell_to_open", "sell_to_close"},
							},
						},
						Required: []string{"symbol", "side", "ratio_qty"},
					},
				},
				"qty": {
					Type:        "string",
					Description: "Number of contracts for the overall order (positive integer)",
				},
				"limit_price": {
					Type:        "string",
					Description: "Net debit (positive) or credit (negative) limit price for the spread",
				},
				"time_in_force": {
					Type:        "string",
					Description: "Time in force: day, gtc, fok, ioc (default: day)",
					Enum:        []string{"day", "gtc", "fok", "ioc"},
				},
			},
			Required: []string{"legs", "qty", "limit_price"},
		},
	},
	{
		Name:        "place_box_spread",
		Description: "Place a box spread (synthetic loan). Buys call spread + put spread at same strikes to lock in fixed payoff at expiration. The debit paid approximates a loan; the spread width is the payoff. Example: buy call@K1, sell call@K2, buy put@K2, sell put@K1.",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"underlying": {
					Type:        "string",
					Description: "Underlying stock symbol (e.g., SPY)",
				},
				"expiration": {
					Type:        "string",
					Description: "Expiration date in YYYY-MM-DD format (e.g., 2026-03-20)",
				},
				"strike_low": {
					Type:        "string",
					Description: "Lower strike price (K1)",
				},
				"strike_high": {
					Type:        "string",
					Description: "Upper strike price (K2)",
				},
				"qty": {
					Type:        "string",
					Description: "Number of contracts (positive integer)",
				},
				"limit_price": {
					Type:        "string",
					Description: "Net debit limit price for the box spread. If omitted, uses natural midpoint from quotes.",
				},
				"time_in_force": {
					Type:        "string",
					Description: "Time in force: day, gtc, fok, ioc (default: day)",
					Enum:        []string{"day", "gtc", "fok", "ioc"},
				},
			},
			Required: []string{"underlying", "expiration", "strike_low", "strike_high", "qty"},
		},
	},
	{
		Name:        "get_option_chain",
		Description: "Get option chain data for an underlying symbol including latest trade, quote, greeks, and implied volatility for each contract",
		InputSchema: JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"underlying": {
					Type:        "string",
					Description: "Underlying stock symbol (e.g., AAPL)",
				},
				"type": {
					Type:        "string",
					Description: "Option type filter: put or call",
					Enum:        []string{"put", "call"},
				},
				"strike_gte": {
					Type:        "string",
					Description: "Minimum strike price",
				},
				"strike_lte": {
					Type:        "string",
					Description: "Maximum strike price",
				},
				"expiration_gte": {
					Type:        "string",
					Description: "Minimum expiration date (YYYY-MM-DD)",
				},
				"expiration_lte": {
					Type:        "string",
					Description: "Maximum expiration date (YYYY-MM-DD)",
				},
				"limit": {
					Type:        "string",
					Description: "Maximum number of contracts to return (default 100)",
				},
			},
			Required: []string{"underlying"},
		},
	},
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size to 1MB to handle large messages
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	encoder := json.NewEncoder(os.Stdout)

	fmt.Fprintln(os.Stderr, "alpaca-mcp: server started")

	for scanner.Scan() {
		line := scanner.Bytes()
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintf(os.Stderr, "alpaca-mcp: json unmarshal error: %v\n", err)
			continue
		}
		fmt.Fprintf(os.Stderr, "alpaca-mcp: recv %s id=%v\n", req.Method, req.ID)

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
		case "notifications/initialized":
			continue
		case "ping":
			resp.Result = map[string]any{}
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

		if err := encoder.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "alpaca-mcp: write error id=%v: %v\n", req.ID, err)
			break
		}
		fmt.Fprintf(os.Stderr, "alpaca-mcp: sent id=%v\n", req.ID)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "alpaca-mcp: scanner error: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "alpaca-mcp: server exiting")
}

func handleToolCall(params ToolCallParams) (result ToolCallResult) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "alpaca-mcp: panic in %s: %v\n", params.Name, r)
			result = ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("internal error: %v", r)}},
				IsError: true,
			}
		}
	}()

	switch params.Name {
	case "place_order":
		return placeOrder(params.Arguments)
	case "get_quote":
		return getQuote(params.Arguments)
	case "get_account":
		return getAccount()
	case "get_positions":
		return getPositions()
	case "get_orders":
		return getOrders(params.Arguments)
	case "cancel_order":
		return cancelOrder(params.Arguments)
	case "cancel_all_orders":
		return cancelAllOrders()
	case "get_bars":
		return getBars(params.Arguments)
	case "get_snapshot":
		return getSnapshot(params.Arguments)
	case "get_auctions":
		return getAuctions(params.Arguments)
	case "get_option_chain":
		return getOptionChain(params.Arguments)
	case "place_mleg_order":
		return placeMlegOrder(params.Arguments)
	case "place_box_spread":
		return placeBoxSpread(params.Arguments)
	default:
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "unknown tool: " + params.Name}},
			IsError: true,
		}
	}
}

func getStr(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getStrSlice(args map[string]any, key string) []string {
	if v, ok := args[key]; ok {
		// Handle array of strings
		if arr, ok := v.([]any); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
		// Handle single string (Claude sometimes sends string instead of array)
		if s, ok := v.(string); ok && s != "" {
			return []string{s}
		}
	}
	return nil
}

func placeOrder(args map[string]any) ToolCallResult {
	symbols := getStrSlice(args, "symbols")

	qtyStr := getStr(args, "qty")
	amtStr := getStr(args, "amt")

	qty := decimal.Zero
	amt := decimal.Zero
	if qtyStr != "" {
		var err error
		qty, err = decimal.ParseString(qtyStr)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid qty: %v", err)}},
				IsError: true,
			}
		}
	}
	if amtStr != "" {
		var err error
		amt, err = decimal.ParseString(amtStr)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid amt: %v", err)}},
				IsError: true,
			}
		}
	}

	orderTypeStr := getStr(args, "order_type")
	if orderTypeStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: `order_type is required. Please explicitly specify your intent:
  - "limit" with greed parameter to auto-price relative to midpoint
  - "limit" with a limit_price if stock moves slowly and you did snapshot analysis
  - "stop" or "stop_limit" for conditional orders
  - "trailing_stop" with trail_price or trail_percent

Trading tips: We prefer patience and good execution over urgency. Consider:
  - Using VWAP/TWAP algorithms for nontrivial equity orders (algorithm: "vwap") we get negative fees and you can trust it to always get great fills even with negative greed
  - For options you can often lowball outside the spread if it's a strike with volume. If it's just MMs trading they might give us better than mid
  - If it fills immediately, then you left money on the table. It's perfectly fine to not get a fill, need to cancel, and then try again`}},
			IsError: true,
		}
	}
	var orderType alpaca.OrderType
	switch orderTypeStr {
	case "limit":
		orderType = alpaca.OrderTypeLimit
	case "stop":
		orderType = alpaca.OrderTypeStop
	case "stop_limit":
		orderType = alpaca.OrderTypeStopLimit
	case "trailing_stop":
		orderType = alpaca.OrderTypeTrailingStop
	default:
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid order_type: %s (use limit, stop, stop_limit, or trailing_stop)", orderTypeStr)}},
			IsError: true,
		}
	}

	limitPrice := decimal.Zero
	if s := getStr(args, "limit_price"); s != "" {
		var err error
		limitPrice, err = decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid limit_price: %v", err)}},
				IsError: true,
			}
		}
	}

	stopPrice := decimal.Zero
	if s := getStr(args, "stop_price"); s != "" {
		var err error
		stopPrice, err = decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid stop_price: %v", err)}},
				IsError: true,
			}
		}
	}

	trailPrice := decimal.Zero
	if s := getStr(args, "trail_price"); s != "" {
		var err error
		trailPrice, err = decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid trail_price: %v", err)}},
				IsError: true,
			}
		}
	}

	trailPercent := decimal.Zero
	if s := getStr(args, "trail_percent"); s != "" {
		var err error
		trailPercent, err = decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid trail_percent: %v", err)}},
				IsError: true,
			}
		}
	}

	greed := decimal.Zero
	if s := getStr(args, "greed"); s != "" {
		g, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid greed: %v", err)}},
				IsError: true,
			}
		}
		greed = g.DivInt(10000)
	}

	tifStr := getStr(args, "time_in_force")
	timeInForce := alpaca.TimeInForceDay
	switch tifStr {
	case "gtc":
		timeInForce = alpaca.TimeInForceGTC
	case "opg":
		timeInForce = alpaca.TimeInForceOPG
	case "cls":
		timeInForce = alpaca.TimeInForceCLS
	case "fok":
		timeInForce = alpaca.TimeInForceFOK
	case "ioc":
		timeInForce = alpaca.TimeInForceIOC
	}

	algoStr := getStr(args, "algorithm")
	algorithm := alpaca.OrderAlgorithmNone
	switch algoStr {
	case "twap":
		algorithm = alpaca.OrderAlgorithmTWAP
	case "vwap":
		algorithm = alpaca.OrderAlgorithmVWAP
	}

	var endTime clocky.Time
	if s := getStr(args, "duration"); s != "" {
		d, err := clocky.ParseDuration(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid duration: %v", err)}},
				IsError: true,
			}
		}
		endTime = clocky.Now().Add(d)
	}

	participation := decimal.FromInt(15).DivInt(100)
	if s := getStr(args, "participation"); s != "" {
		var err error
		participation, err = decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid participation: %v", err)}},
				IsError: true,
			}
		}
	}

	extendedHours := getStr(args, "extended_hours") == "true"

	// Bracket order stop loss (separate from stop_price used for stop order types)
	var stopLoss *alpaca.StopLoss
	if s := getStr(args, "bracket_stop_price"); s != "" {
		sp, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid bracket_stop_price: %v", err)}},
				IsError: true,
			}
		}
		stopLoss = &alpaca.StopLoss{StopPrice: sp}
		if sl := getStr(args, "bracket_stop_limit_price"); sl != "" {
			lp, err := decimal.ParseString(sl)
			if err != nil {
				return ToolCallResult{
					Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid bracket_stop_limit_price: %v", err)}},
					IsError: true,
				}
			}
			stopLoss.LimitPrice = lp
		}
	}

	var takeProfit *alpaca.TakeProfit
	if s := getStr(args, "take_profit_price"); s != "" {
		tp, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid take_profit_price: %v", err)}},
				IsError: true,
			}
		}
		takeProfit = &alpaca.TakeProfit{LimitPrice: tp}
	}

	orderClass := alpaca.OrderClassSimple
	switch getStr(args, "order_class") {
	case "bracket":
		orderClass = alpaca.OrderClassBracket
	case "oco":
		orderClass = alpaca.OrderClassOCO
	case "oto":
		orderClass = alpaca.OrderClassOTO
	case "mleg":
		orderClass = alpaca.OrderClassMleg
	}

	client := alpaca.NewClient()

	// Handle mleg orders: legs define the symbols/sides, not the top-level fields
	if orderClass == alpaca.OrderClassMleg {
		return placeMlegViaPlaceOrder(args, client, qty, limitPrice, orderType, timeInForce)
	}

	var results []string

	if len(symbols) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no symbols specified"}},
			IsError: true,
		}
	}
	if qtyStr == "" && amtStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "must specify qty or amt"}},
			IsError: true,
		}
	}

	for _, sym := range symbols {
		// Get bid/ask prices - handle options vs stocks differently
		var bidPrice, askPrice decimal.Decimal
		if isOptionSymbol(sym) {
			snap, err := client.GetOptionSnapshot(sym)
			if err != nil {
				results = append(results, fmt.Sprintf("%s: error getting option quote: %v", sym, err))
				continue
			}
			if snap.LatestQuote == nil {
				results = append(results, fmt.Sprintf("%s: no quote available", sym))
				continue
			}
			bidPrice = snap.LatestQuote.BidPrice
			askPrice = snap.LatestQuote.AskPrice
		} else {
			quote, err := client.GetQuote(sym)
			if err != nil {
				results = append(results, fmt.Sprintf("%s: error getting quote: %v", sym, err))
				continue
			}
			bidPrice = quote.BidPrice
			askPrice = quote.AskPrice
		}

		side := ds.SideBuy
		orderQty := qty
		if !orderQty.IsZero() {
			if orderQty.IsNegative() {
				side = ds.SideSell
				orderQty = orderQty.Neg()
			}
		} else {
			orderAmt := amt
			if orderAmt.IsNegative() {
				side = ds.SideSell
				orderAmt = orderAmt.Neg()
				orderQty = orderAmt.Div(bidPrice).Truncate()
			} else {
				orderQty = orderAmt.Div(askPrice).Truncate()
			}
			if orderQty.IsZero() {
				results = append(results, fmt.Sprintf("%s: amount too small for any shares", sym))
				continue
			}
		}

		orderLimitPrice := limitPrice
		if orderType == alpaca.OrderTypeLimit && orderLimitPrice.IsZero() {
			orderLimitPrice = bidPrice.Add(askPrice).DivInt(2)
			if side == ds.SideBuy {
				orderLimitPrice = orderLimitPrice.Mul(decimal.One.Sub(greed))
				orderLimitPrice = orderLimitPrice.QuantizeTruncate(decimal.Cent)
			} else {
				orderLimitPrice = orderLimitPrice.Mul(decimal.One.Add(greed))
				orderLimitPrice = orderLimitPrice.QuantizeAway(decimal.Cent)
			}
		}

		order, err := client.CreateOrder(&alpaca.OrderRequest{
			Symbol:        sym,
			Side:          side,
			Qty:           orderQty,
			LimitPrice:    orderLimitPrice,
			StopPrice:     stopPrice,
			TrailPrice:    trailPrice,
			TrailPercent:  trailPercent,
			Type:          orderType,
			TimeInForce:   timeInForce,
			ExtendedHours: extendedHours,
			OrderClass:    orderClass,
			StopLoss:      stopLoss,
			TakeProfit:    takeProfit,
			AdvancedInstructions: &alpaca.AdvancedInstructions{
				Algorithm:     algorithm,
				EndTime:       endTime,
				MaxPercentage: participation,
			},
		})
		if err != nil {
			results = append(results, fmt.Sprintf("%s: error placing order: %v", sym, err))
			continue
		}

		var priceStr string
		switch orderType {
		case alpaca.OrderTypeLimit:
			priceStr = fmt.Sprintf("limit %s", orderLimitPrice)
		case alpaca.OrderTypeStop:
			priceStr = fmt.Sprintf("stop %s", stopPrice)
		case alpaca.OrderTypeStopLimit:
			priceStr = fmt.Sprintf("stop %s limit %s", stopPrice, orderLimitPrice)
		case alpaca.OrderTypeTrailingStop:
			if !trailPercent.IsZero() {
				priceStr = fmt.Sprintf("trail %s%%", trailPercent)
			} else {
				priceStr = fmt.Sprintf("trail $%s", trailPrice)
			}
		default:
			priceStr = "market"
		}
		results = append(results, fmt.Sprintf("%s: ordered %s %s at %s (id: %s)", sym, side, orderQty, priceStr, order.ID))
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(results, "\n")}},
	}
}

func placeMlegViaPlaceOrder(args map[string]any, client *alpaca.Client, qty decimal.Decimal, limitPrice decimal.Decimal, orderType alpaca.OrderType, timeInForce alpaca.TimeInForce) ToolCallResult {
	legs := getMapSlice(args, "legs")
	if len(legs) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "legs array is required for mleg orders"}},
			IsError: true,
		}
	}
	if len(legs) > 4 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("max 4 legs allowed, got %d", len(legs))}},
			IsError: true,
		}
	}
	if !qty.IsPositive() {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "qty must be a positive number for mleg orders"}},
			IsError: true,
		}
	}

	var orderLegs []alpaca.OrderLeg
	for i, leg := range legs {
		sym := getStr(leg, "symbol")
		if sym == "" {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: symbol is required", i+1)}},
				IsError: true,
			}
		}
		if !isOptionSymbol(sym) {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: invalid OCC option symbol: %s", i+1, sym)}},
				IsError: true,
			}
		}

		sideStr := getStr(leg, "side")
		if sideStr == "" {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: side is required (buy or sell)", i+1)}},
				IsError: true,
			}
		}
		var side ds.Side
		switch sideStr {
		case "buy":
			side = ds.SideBuy
		case "sell":
			side = ds.SideSell
		default:
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: side must be buy or sell, got %s", i+1, sideStr)}},
				IsError: true,
			}
		}

		// ratio_qty may come as string or float64 from JSON
		ratioQty := decimal.One
		if v, ok := leg["ratio_qty"]; ok {
			switch r := v.(type) {
			case string:
				if parsed, err := decimal.ParseString(r); err == nil && parsed.IsPositive() {
					ratioQty = parsed
				}
			case float64:
				if r > 0 {
					ratioQty = decimal.FromInt(int(r))
				}
			}
		}

		var posIntent alpaca.PositionIntent
		switch getStr(leg, "position_intent") {
		case "buy_to_open":
			posIntent = alpaca.PositionIntentBuyToOpen
		case "buy_to_close":
			posIntent = alpaca.PositionIntentBuyToClose
		case "sell_to_open":
			posIntent = alpaca.PositionIntentSellToOpen
		case "sell_to_close":
			posIntent = alpaca.PositionIntentSellToClose
		}

		orderLegs = append(orderLegs, alpaca.OrderLeg{
			Symbol:         sym,
			Side:           side,
			RatioQty:       ratioQty,
			PositionIntent: posIntent,
		})
	}

	// Fetch quotes for all legs
	var quoteLines []string
	for _, leg := range orderLegs {
		snap, err := client.GetOptionSnapshot(leg.Symbol)
		if err != nil {
			quoteLines = append(quoteLines, fmt.Sprintf("  %s: error getting quote: %v", leg.Symbol, err))
			continue
		}
		if snap.LatestQuote != nil {
			q := snap.LatestQuote
			mid := q.BidPrice.Add(q.AskPrice).DivInt(2)
			quoteLines = append(quoteLines, fmt.Sprintf("  %s (%s): bid %s ask %s mid %s",
				leg.Symbol, leg.Side, q.BidPrice, q.AskPrice, mid))
		} else {
			quoteLines = append(quoteLines, fmt.Sprintf("  %s: no quote available", leg.Symbol))
		}
	}

	order, err := client.CreateOrder(&alpaca.OrderRequest{
		Qty:         qty,
		Type:        orderType,
		TimeInForce: timeInForce,
		LimitPrice:  limitPrice,
		OrderClass:  alpaca.OrderClassMleg,
		Legs:        orderLegs,
	})
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error placing mleg order: %v\n\nQuotes:\n%s", err, join(quoteLines, "\n"))}},
			IsError: true,
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("mleg order placed (id: %s)", order.ID))
	lines = append(lines, fmt.Sprintf("  qty: %s  limit: %s  tif: %s", qty, limitPrice, timeInForce))
	lines = append(lines, "  legs:")
	for _, leg := range orderLegs {
		lines = append(lines, fmt.Sprintf("    %s %s ratio %s", leg.Side, leg.Symbol, leg.RatioQty))
	}
	lines = append(lines, "\nQuotes:")
	lines = append(lines, quoteLines...)

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func getQuote(args map[string]any) ToolCallResult {
	symbol := getStr(args, "symbol")
	if symbol == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no symbol specified"}},
			IsError: true,
		}
	}

	client := alpaca.NewClient()

	// Handle options symbols differently
	if isOptionSymbol(symbol) {
		snap, err := client.GetOptionSnapshot(symbol)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
				IsError: true,
			}
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("%s (option) quote:", symbol))

		if snap.LatestQuote != nil {
			q := snap.LatestQuote
			midpoint := q.BidPrice.Add(q.AskPrice).DivInt(2)
			spread := q.AskPrice.Sub(q.BidPrice)
			lines = append(lines, fmt.Sprintf("  Bid: %s (size: %d)", q.BidPrice, q.BidSize))
			lines = append(lines, fmt.Sprintf("  Ask: %s (size: %d)", q.AskPrice, q.AskSize))
			lines = append(lines, fmt.Sprintf("  Midpoint: %s  Spread: %s", midpoint, spread))
		}

		if !snap.ImpliedVolatility.IsZero() {
			lines = append(lines, fmt.Sprintf("  IV: %s%%", snap.ImpliedVolatility.MulInt(100).Format(2)))
		}

		if snap.Greeks != nil {
			g := snap.Greeks
			lines = append(lines, fmt.Sprintf("  Greeks: Δ=%s Γ=%s Θ=%s V=%s ρ=%s",
				g.Delta.Format(4), g.Gamma.Format(4), g.Theta.Format(4), g.Vega.Format(4), g.Rho.Format(4)))
		}

		return ToolCallResult{
			Content: []Content{{Type: "text", Text: join(lines, "\n")}},
		}
	}

	// Stock quote
	quote, err := client.GetQuote(symbol)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	midpoint := quote.BidPrice.Add(quote.AskPrice).DivInt(2)
	spread := quote.AskPrice.Sub(quote.BidPrice)
	spreadBps := decimal.Zero
	if !midpoint.IsZero() {
		spreadBps = spread.Div(midpoint).MulInt(10000)
	}

	text := fmt.Sprintf("%s quote:\n  Bid: %s (size: %d)\n  Ask: %s (size: %d)\n  Midpoint: %s\n  Spread: %s (%s bps)",
		symbol,
		quote.BidPrice, quote.BidSize,
		quote.AskPrice, quote.AskSize,
		midpoint,
		spread, spreadBps.Format(1))

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: text}},
	}
}

func getAccount() ToolCallResult {
	client := alpaca.NewClient()
	acct, err := client.GetAccount()
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	var lines []string
	lines = append(lines, "Account Info:")
	lines = append(lines, fmt.Sprintf("  Account ID: %s", acct.ID))
	lines = append(lines, fmt.Sprintf("  Status: %s", acct.Status))
	lines = append(lines, fmt.Sprintf("  Currency: %s", acct.Currency))
	lines = append(lines, "")
	lines = append(lines, "Balances:")
	lines = append(lines, fmt.Sprintf("  Cash: $%s", acct.Cash))
	lines = append(lines, fmt.Sprintf("  Equity: $%s", acct.Equity))
	lines = append(lines, fmt.Sprintf("  Last Equity: $%s", acct.LastEquity))
	lines = append(lines, fmt.Sprintf("  Long Market Value: $%s", acct.LongMarketValue))
	lines = append(lines, fmt.Sprintf("  Short Market Value: $%s", acct.ShortMarketValue))
	lines = append(lines, "")
	lines = append(lines, "Buying Power:")
	lines = append(lines, fmt.Sprintf("  Buying Power: $%s", acct.BuyingPower))
	lines = append(lines, fmt.Sprintf("  Daytrading BP: $%s", acct.DaytradingBuyingPower))
	lines = append(lines, fmt.Sprintf("  RegT BP: $%s", acct.RegTBuyingPower))
	lines = append(lines, fmt.Sprintf("  Non-Margin BP: $%s", acct.NonMarginBuyingPower))
	lines = append(lines, "")
	lines = append(lines, "Margin:")
	lines = append(lines, fmt.Sprintf("  Multiplier: %s", acct.Multiplier))
	lines = append(lines, fmt.Sprintf("  Initial Margin: $%s", acct.InitialMargin))
	lines = append(lines, fmt.Sprintf("  Maintenance Margin: $%s", acct.MaintenanceMargin))
	lines = append(lines, fmt.Sprintf("  SMA: $%s", acct.SMA))
	lines = append(lines, "")
	lines = append(lines, "Day Trading:")
	lines = append(lines, fmt.Sprintf("  Pattern Day Trader: %t", acct.PatternDayTrader))
	lines = append(lines, fmt.Sprintf("  Day Trade Count (5d): %d", acct.DaytradeCount))
	lines = append(lines, "")
	lines = append(lines, "Permissions:")
	lines = append(lines, fmt.Sprintf("  Shorting Enabled: %t", acct.ShortingEnabled))
	lines = append(lines, fmt.Sprintf("  Trading Blocked: %t", acct.TradingBlocked))

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func getPositions() ToolCallResult {
	client := alpaca.NewClient()
	positions, err := client.GetPositions()
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	if len(positions) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no open positions"}},
		}
	}

	var lines []string
	lines = append(lines, "Open positions:")
	for _, p := range positions {
		lines = append(lines, fmt.Sprintf("  %s: %s shares @ %s avg cost (market: %s, P&L: %s)",
			p.Symbol, p.Qty, p.AvgEntryPrice, p.CurrentPrice, p.UnrealizedPL))
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func getOrders(args map[string]any) ToolCallResult {
	client := alpaca.NewClient()

	var req *alpaca.GetOrdersRequest
	status := getStr(args, "status")
	limitStr := getStr(args, "limit")
	symbols := getStr(args, "symbols")
	direction := getStr(args, "direction")

	if status != "" || limitStr != "" || symbols != "" || direction != "" {
		req = &alpaca.GetOrdersRequest{
			Status:    status,
			Direction: direction,
		}
		if limitStr != "" {
			n, err := decimal.ParseString(limitStr)
			if err != nil {
				return ToolCallResult{
					Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid limit: %v", err)}},
					IsError: true,
				}
			}
			req.Limit = n.Int()
		}
		if symbols != "" {
			req.Symbols = strings.Split(symbols, ",")
		}
	}

	orders, err := client.GetOrders(req)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	if len(orders) == 0 {
		statusLabel := "open"
		if status != "" {
			statusLabel = status
		}
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("no %s orders", statusLabel)}},
		}
	}

	var lines []string
	statusLabel := "Open"
	switch status {
	case "closed":
		statusLabel = "Closed"
	case "all":
		statusLabel = "All"
	}
	lines = append(lines, fmt.Sprintf("%s orders (%d):", statusLabel, len(orders)))
	for _, o := range orders {
		lines = append(lines, fmt.Sprintf("  %s: %s %s %s @ %s (status: %s, id: %s)",
			o.Symbol, o.Side, o.Qty, o.Type, o.LimitPrice, o.Status, o.ID))
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func cancelOrder(args map[string]any) ToolCallResult {
	orderID := getStr(args, "order_id")
	if orderID == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no order_id specified"}},
			IsError: true,
		}
	}

	client := alpaca.NewClient()
	err := client.CancelOrder(orderID)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: fmt.Sprintf("order %s cancelled", orderID)}},
	}
}

func cancelAllOrders() ToolCallResult {
	client := alpaca.NewClient()
	err := client.CancelAllOrders()
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: "all orders cancelled"}},
	}
}

func getBars(args map[string]any) ToolCallResult {
	symbol, err := symbol.Parse(getStr(args, "symbol"))
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "bad or missing equity symbol specified"}},
			IsError: true,
		}
	}

	// Parse timeframe (default to 1 day)
	timeframe := clocky.Day
	if tf := getStr(args, "timeframe"); tf != "" {
		switch tf {
		case "1m":
			timeframe = clocky.Minute
		case "5m":
			timeframe = 5 * clocky.Minute
		case "15m":
			timeframe = 15 * clocky.Minute
		case "30m":
			timeframe = 30 * clocky.Minute
		case "1h":
			timeframe = clocky.Hour
		case "4h":
			timeframe = 4 * clocky.Hour
		case "1d":
			timeframe = clocky.Day
		case "1w":
			timeframe = clocky.Week
		case "1M":
			timeframe = clocky.Monthy
		}
	}

	// Parse start/end times
	var start, end clocky.Time
	if s := getStr(args, "start"); s != "" {
		var err error
		start, err = clocky.ParseTime(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid start time: %v", err)}},
				IsError: true,
			}
		}
	}
	if s := getStr(args, "end"); s != "" {
		var err error
		end, err = clocky.ParseTime(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid end time: %v", err)}},
				IsError: true,
			}
		}
	}

	// Parse limit (default 100)
	limit := 100
	if s := getStr(args, "limit"); s != "" {
		n, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid limit: %v", err)}},
				IsError: true,
			}
		}
		if n.Int() > 0 && n.Int() <= 10000 {
			limit = n.Int()
		}
	}

	// Parse feed
	var feed alpaca.DataFeed
	switch getStr(args, "feed") {
	case "iex":
		feed = alpaca.DataFeedIEX
	case "otc":
		feed = alpaca.DataFeedOTC
	default:
		feed = alpaca.DataFeedSIP
	}

	// Parse adjustment
	var adjustment alpaca.BarAdjustment
	switch getStr(args, "adjustment") {
	case "split":
		adjustment = alpaca.BarAdjustmentSplit
	case "dividend":
		adjustment = alpaca.BarAdjustmentDividend
	case "all":
		adjustment = alpaca.BarAdjustmentAll
	default:
		adjustment = alpaca.BarAdjustmentRaw
	}

	client := alpaca.NewClient()
	bars, _, err := client.GetBars(symbol, timeframe, start, end, feed, adjustment, limit, false, "")
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	if len(bars) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("no bars found for %s", symbol)}},
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s bars (%d):", symbol, len(bars)))
	lines = append(lines, "Time                     Open       High       Low        Close      Volume     VWAP")
	for _, b := range bars {
		lines = append(lines, fmt.Sprintf("%s  %10s %10s %10s %10s %10s %10s",
			b.Timestamp.RFC3339NYC(),
			b.Open.Format(2),
			b.High.Format(2),
			b.Low.Format(2),
			b.Close.Format(2),
			b.Volume.String(),
			b.VWAP.Format(2)))
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func getSnapshot(args map[string]any) ToolCallResult {
	symbol := getStr(args, "symbol")
	if symbol == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no symbol specified"}},
			IsError: true,
		}
	}

	client := alpaca.NewClient()

	// Handle options symbols differently
	if isOptionSymbol(symbol) {
		snap, err := client.GetOptionSnapshot(symbol)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
				IsError: true,
			}
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("%s (option) snapshot:", symbol))

		if snap.LatestTrade != nil {
			t := snap.LatestTrade
			lines = append(lines, "\nLatest Trade:")
			lines = append(lines, fmt.Sprintf("  Price: %s  Size: %d  Exchange: %s  Time: %s",
				t.Price, t.Size, t.Exchange, t.Timestamp.RFC3339NYC()))
		}

		if snap.LatestQuote != nil {
			q := snap.LatestQuote
			midpoint := q.BidPrice.Add(q.AskPrice).DivInt(2)
			spread := q.AskPrice.Sub(q.BidPrice)
			lines = append(lines, "\nLatest Quote:")
			lines = append(lines, fmt.Sprintf("  Bid: %s (size: %d)  Ask: %s (size: %d)",
				q.BidPrice, q.BidSize, q.AskPrice, q.AskSize))
			lines = append(lines, fmt.Sprintf("  Midpoint: %s  Spread: %s", midpoint, spread))
		}

		if !snap.ImpliedVolatility.IsZero() {
			lines = append(lines, fmt.Sprintf("\nImplied Volatility: %s%%", snap.ImpliedVolatility.MulInt(100).Format(2)))
		}

		if snap.Greeks != nil {
			g := snap.Greeks
			lines = append(lines, "\nGreeks:")
			lines = append(lines, fmt.Sprintf("  Delta: %s  Gamma: %s  Theta: %s  Vega: %s  Rho: %s",
				g.Delta.Format(4), g.Gamma.Format(4), g.Theta.Format(4), g.Vega.Format(4), g.Rho.Format(4)))
		}

		return ToolCallResult{
			Content: []Content{{Type: "text", Text: join(lines, "\n")}},
		}
	}

	// Stock snapshot
	snapshot, err := client.GetSnapshot(symbol)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s snapshot:", symbol))

	if snapshot.LatestTrade != nil {
		t := snapshot.LatestTrade
		lines = append(lines, "\nLatest Trade:")
		lines = append(lines, fmt.Sprintf("  Price: %s  Size: %d  Exchange: %s  Time: %s",
			t.Price, t.Size, t.Exchange, t.Timestamp.RFC3339NYC()))
	}

	if snapshot.LatestQuote != nil {
		q := snapshot.LatestQuote
		midpoint := q.BidPrice.Add(q.AskPrice).DivInt(2)
		spread := q.AskPrice.Sub(q.BidPrice)
		lines = append(lines, "\nLatest Quote:")
		lines = append(lines, fmt.Sprintf("  Bid: %s (size: %s)  Ask: %s (size: %s)",
			q.BidPrice, q.BidSize, q.AskPrice, q.AskSize))
		lines = append(lines, fmt.Sprintf("  Midpoint: %s  Spread: %s", midpoint, spread))
	}

	if snapshot.MinuteBar != nil {
		b := snapshot.MinuteBar
		lines = append(lines, fmt.Sprintf("\nMinute Bar (%s):", b.Timestamp.RFC3339NYC()))
		lines = append(lines, fmt.Sprintf("  O: %s  H: %s  L: %s  C: %s  V: %s  VWAP: %s",
			b.Open.Format(2), b.High.Format(2), b.Low.Format(2), b.Close.Format(2), b.Volume, b.VWAP.Format(2)))
	}

	if snapshot.DailyBar != nil {
		b := snapshot.DailyBar
		lines = append(lines, fmt.Sprintf("\nDaily Bar (%s):", b.Timestamp.RFC3339NYC()))
		lines = append(lines, fmt.Sprintf("  O: %s  H: %s  L: %s  C: %s  V: %s  VWAP: %s",
			b.Open.Format(2), b.High.Format(2), b.Low.Format(2), b.Close.Format(2), b.Volume, b.VWAP.Format(2)))
	}

	if snapshot.PrevDailyBar != nil {
		b := snapshot.PrevDailyBar
		lines = append(lines, fmt.Sprintf("\nPrevious Daily Bar (%s):", b.Timestamp.RFC3339NYC()))
		lines = append(lines, fmt.Sprintf("  O: %s  H: %s  L: %s  C: %s  V: %s  VWAP: %s",
			b.Open.Format(2), b.High.Format(2), b.Low.Format(2), b.Close.Format(2), b.Volume, b.VWAP.Format(2)))
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func getAuctions(args map[string]any) ToolCallResult {
	symbol := getStr(args, "symbol")
	if symbol == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no symbol specified"}},
			IsError: true,
		}
	}

	// Parse start/end times
	var start, end clocky.Time
	if s := getStr(args, "start"); s != "" {
		var err error
		start, err = clocky.ParseTime(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid start time: %v", err)}},
				IsError: true,
			}
		}
	}
	if s := getStr(args, "end"); s != "" {
		var err error
		end, err = clocky.ParseTime(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid end time: %v", err)}},
				IsError: true,
			}
		}
	}

	// Parse limit (default 100)
	limit := 100
	if s := getStr(args, "limit"); s != "" {
		n, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid limit: %v", err)}},
				IsError: true,
			}
		}
		if n.Int() > 0 && n.Int() <= 10000 {
			limit = n.Int()
		}
	}

	// Parse feed
	var feed alpaca.DataFeed
	switch getStr(args, "feed") {
	case "iex":
		feed = alpaca.DataFeedIEX
	case "otc":
		feed = alpaca.DataFeedOTC
	default:
		feed = alpaca.DataFeedSIP
	}

	client := alpaca.NewClient()
	auctions, _, err := client.GetAuctions(symbol, start, end, feed, limit, false, "")
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	if len(auctions) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("no auctions found for %s", symbol)}},
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s auctions (%d days):", symbol, len(auctions)))

	for _, a := range auctions {
		lines = append(lines, fmt.Sprintf("\n%s:", a.Date))
		if len(a.Opening) > 0 {
			lines = append(lines, "  Opening:")
			for _, p := range a.Opening {
				lines = append(lines, fmt.Sprintf("    %s @ %s (size: %d, exchange: %s)",
					p.Timestamp.RFC3339NYC(), p.Price, p.Size, p.Exchange))
			}
		}
		if len(a.Closing) > 0 {
			lines = append(lines, "  Closing:")
			for _, p := range a.Closing {
				lines = append(lines, fmt.Sprintf("    %s @ %s (size: %d, exchange: %s)",
					p.Timestamp.RFC3339NYC(), p.Price, p.Size, p.Exchange))
			}
		}
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func getOptionChain(args map[string]any) ToolCallResult {
	underlying := getStr(args, "underlying")
	if underlying == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no underlying symbol specified"}},
			IsError: true,
		}
	}

	req := &alpaca.OptionChainRequest{}

	if t := getStr(args, "type"); t != "" {
		req.Type = t
	}
	if s := getStr(args, "strike_gte"); s != "" {
		v, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid strike_gte: %v", err)}},
				IsError: true,
			}
		}
		req.StrikeGTE = v
	}
	if s := getStr(args, "strike_lte"); s != "" {
		v, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid strike_lte: %v", err)}},
				IsError: true,
			}
		}
		req.StrikeLTE = v
	}
	if s := getStr(args, "expiration_gte"); s != "" {
		v, err := clocky.ParseTime(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid expiration_gte: %v", err)}},
				IsError: true,
			}
		}
		req.ExpirationGTE = v
	}
	if s := getStr(args, "expiration_lte"); s != "" {
		v, err := clocky.ParseTime(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid expiration_lte: %v", err)}},
				IsError: true,
			}
		}
		req.ExpirationLTE = v
	}
	if s := getStr(args, "limit"); s != "" {
		n, err := decimal.ParseString(s)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid limit: %v", err)}},
				IsError: true,
			}
		}
		if n.Int() > 0 {
			req.Limit = n.Int()
		}
	}

	client := alpaca.NewClient()
	chain, _, err := client.GetOptionChain(underlying, req)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error: %v", err)}},
			IsError: true,
		}
	}

	if len(chain) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("no options found for %s", underlying)}},
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s option chain (%d contracts):", underlying, len(chain)))
	lines = append(lines, "")

	for symbol, snap := range chain {
		lines = append(lines, fmt.Sprintf("%s:", symbol))

		if snap.LatestQuote != nil {
			q := snap.LatestQuote
			lines = append(lines, fmt.Sprintf("  Quote: %s x %d / %s x %d",
				q.BidPrice, q.BidSize, q.AskPrice, q.AskSize))
		}

		if snap.LatestTrade != nil {
			t := snap.LatestTrade
			lines = append(lines, fmt.Sprintf("  Last: %s (size: %d) @ %s",
				t.Price, t.Size, t.Timestamp.RFC3339NYC()))
		}

		if !snap.ImpliedVolatility.IsZero() {
			lines = append(lines, fmt.Sprintf("  IV: %s%%", snap.ImpliedVolatility.MulInt(100).Format(2)))
		}

		if snap.Greeks != nil {
			g := snap.Greeks
			lines = append(lines, fmt.Sprintf("  Greeks: Δ=%s Γ=%s Θ=%s V=%s ρ=%s",
				g.Delta.Format(4), g.Gamma.Format(4), g.Theta.Format(4), g.Vega.Format(4), g.Rho.Format(4)))
		}

		lines = append(lines, "")
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func getMapSlice(args map[string]any, key string) []map[string]any {
	v, ok := args[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

// buildOCCSymbol constructs an OCC option symbol from components.
// root: underlying symbol (1-6 uppercase letters)
// expiration: date in YYYY-MM-DD format
// optionType: "C" or "P"
// strike: strike price as decimal string (e.g., "500", "12.50")
func buildOCCSymbol(root, expiration, optionType, strike string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("root symbol is required")
	}
	if len(root) > 6 {
		return "", fmt.Errorf("root symbol too long: %s", root)
	}
	for _, c := range root {
		if c < 'A' || c > 'Z' {
			return "", fmt.Errorf("root symbol must be uppercase letters: %s", root)
		}
	}
	if !occDateRegex.MatchString(expiration) {
		return "", fmt.Errorf("expiration must be YYYY-MM-DD format: %s", expiration)
	}
	if optionType != "C" && optionType != "P" {
		return "", fmt.Errorf("option type must be C or P: %s", optionType)
	}
	strikeD, err := decimal.ParseString(strike)
	if err != nil {
		return "", fmt.Errorf("invalid strike price: %s", strike)
	}
	if !strikeD.IsPositive() {
		return "", fmt.Errorf("strike price must be positive: %s", strike)
	}
	// OCC strike is strike * 1000, zero-padded to 8 digits
	strikeInt := strikeD.MulInt(1000).Int64()
	// Format: ROOT + YYMMDD + C/P + 8-digit strike
	dateStr := expiration[2:4] + expiration[5:7] + expiration[8:10]
	return fmt.Sprintf("%s%s%s%08d", root, dateStr, optionType, strikeInt), nil
}

func placeMlegOrder(args map[string]any) ToolCallResult {
	legs := getMapSlice(args, "legs")
	if len(legs) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "legs array is required and must contain at least one leg"}},
			IsError: true,
		}
	}
	if len(legs) > 4 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("max 4 legs allowed, got %d", len(legs))}},
			IsError: true,
		}
	}

	qtyStr := getStr(args, "qty")
	if qtyStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "qty is required"}},
			IsError: true,
		}
	}
	qty, err := decimal.ParseString(qtyStr)
	if err != nil || !qty.IsPositive() {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "qty must be a positive number"}},
			IsError: true,
		}
	}

	limitStr := getStr(args, "limit_price")
	if limitStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "limit_price is required for mleg orders"}},
			IsError: true,
		}
	}
	limitPrice, err := decimal.ParseString(limitStr)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid limit_price: %v", err)}},
			IsError: true,
		}
	}

	var orderLegs []alpaca.OrderLeg
	for i, leg := range legs {
		sym := getStr(leg, "symbol")
		if sym == "" {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: symbol is required", i+1)}},
				IsError: true,
			}
		}
		if !isOptionSymbol(sym) {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: invalid OCC option symbol: %s", i+1, sym)}},
				IsError: true,
			}
		}

		sideStr := getStr(leg, "side")
		if sideStr == "" {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: side is required (buy or sell)", i+1)}},
				IsError: true,
			}
		}
		var side ds.Side
		switch sideStr {
		case "buy":
			side = ds.SideBuy
		case "sell":
			side = ds.SideSell
		default:
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: side must be buy or sell, got %s", i+1, sideStr)}},
				IsError: true,
			}
		}

		ratioStr := getStr(leg, "ratio_qty")
		ratioQty := decimal.One
		if ratioStr != "" {
			ratioQty, err = decimal.ParseString(ratioStr)
			if err != nil || !ratioQty.IsPositive() {
				return ToolCallResult{
					Content: []Content{{Type: "text", Text: fmt.Sprintf("leg %d: ratio_qty must be a positive number", i+1)}},
					IsError: true,
				}
			}
		}

		var posIntent alpaca.PositionIntent
		switch getStr(leg, "position_intent") {
		case "buy_to_open":
			posIntent = alpaca.PositionIntentBuyToOpen
		case "buy_to_close":
			posIntent = alpaca.PositionIntentBuyToClose
		case "sell_to_open":
			posIntent = alpaca.PositionIntentSellToOpen
		case "sell_to_close":
			posIntent = alpaca.PositionIntentSellToClose
		}

		orderLegs = append(orderLegs, alpaca.OrderLeg{
			Symbol:         sym,
			Side:           side,
			RatioQty:       ratioQty,
			PositionIntent: posIntent,
		})
	}

	tifStr := getStr(args, "time_in_force")
	timeInForce := alpaca.TimeInForceDay
	switch tifStr {
	case "gtc":
		timeInForce = alpaca.TimeInForceGTC
	case "fok":
		timeInForce = alpaca.TimeInForceFOK
	case "ioc":
		timeInForce = alpaca.TimeInForceIOC
	}

	// Fetch quotes for all legs
	client := alpaca.NewClient()
	var quoteLines []string
	for _, leg := range orderLegs {
		snap, err := client.GetOptionSnapshot(leg.Symbol)
		if err != nil {
			quoteLines = append(quoteLines, fmt.Sprintf("  %s: error getting quote: %v", leg.Symbol, err))
			continue
		}
		if snap.LatestQuote != nil {
			q := snap.LatestQuote
			mid := q.BidPrice.Add(q.AskPrice).DivInt(2)
			quoteLines = append(quoteLines, fmt.Sprintf("  %s: bid %s ask %s mid %s (side: %s, ratio: %s)",
				leg.Symbol, q.BidPrice, q.AskPrice, mid, leg.Side, leg.RatioQty))
		} else {
			quoteLines = append(quoteLines, fmt.Sprintf("  %s: no quote available", leg.Symbol))
		}
	}

	order, err := client.CreateOrder(&alpaca.OrderRequest{
		Qty:         qty,
		Type:        alpaca.OrderTypeLimit,
		TimeInForce: timeInForce,
		LimitPrice:  limitPrice,
		OrderClass:  alpaca.OrderClassMleg,
		Legs:        orderLegs,
	})
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error placing mleg order: %v\n\nQuotes:\n%s", err, join(quoteLines, "\n"))}},
			IsError: true,
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("mleg order placed (id: %s)", order.ID))
	lines = append(lines, fmt.Sprintf("  qty: %s  limit: %s  tif: %s", qty, limitPrice, timeInForce))
	lines = append(lines, "  legs:")
	for _, leg := range orderLegs {
		lines = append(lines, fmt.Sprintf("    %s %s ratio %s", leg.Side, leg.Symbol, leg.RatioQty))
	}
	lines = append(lines, "\nQuotes:")
	lines = append(lines, quoteLines...)

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func placeBoxSpread(args map[string]any) ToolCallResult {
	underlying := getStr(args, "underlying")
	if underlying == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "underlying symbol is required"}},
			IsError: true,
		}
	}

	expiration := getStr(args, "expiration")
	if expiration == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "expiration date is required (YYYY-MM-DD)"}},
			IsError: true,
		}
	}

	strikeLowStr := getStr(args, "strike_low")
	if strikeLowStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "strike_low is required"}},
			IsError: true,
		}
	}
	strikeLow, err := decimal.ParseString(strikeLowStr)
	if err != nil || !strikeLow.IsPositive() {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "strike_low must be a positive number"}},
			IsError: true,
		}
	}

	strikeHighStr := getStr(args, "strike_high")
	if strikeHighStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "strike_high is required"}},
			IsError: true,
		}
	}
	strikeHigh, err := decimal.ParseString(strikeHighStr)
	if err != nil || !strikeHigh.IsPositive() {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "strike_high must be a positive number"}},
			IsError: true,
		}
	}

	if strikeLow.Cmp(strikeHigh) >= 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("strike_low (%s) must be less than strike_high (%s)", strikeLow, strikeHigh)}},
			IsError: true,
		}
	}

	qtyStr := getStr(args, "qty")
	if qtyStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "qty is required"}},
			IsError: true,
		}
	}
	qty, err := decimal.ParseString(qtyStr)
	if err != nil || !qty.IsPositive() {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "qty must be a positive number"}},
			IsError: true,
		}
	}

	// Build OCC symbols for the 4 legs
	buyCallSym, err := buildOCCSymbol(underlying, expiration, "C", strikeLowStr)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error building buy call symbol: %v", err)}},
			IsError: true,
		}
	}
	sellCallSym, err := buildOCCSymbol(underlying, expiration, "C", strikeHighStr)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error building sell call symbol: %v", err)}},
			IsError: true,
		}
	}
	buyPutSym, err := buildOCCSymbol(underlying, expiration, "P", strikeHighStr)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error building buy put symbol: %v", err)}},
			IsError: true,
		}
	}
	sellPutSym, err := buildOCCSymbol(underlying, expiration, "P", strikeLowStr)
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error building sell put symbol: %v", err)}},
			IsError: true,
		}
	}

	orderLegs := []alpaca.OrderLeg{
		{Symbol: buyCallSym, Side: ds.SideBuy, RatioQty: decimal.One, PositionIntent: alpaca.PositionIntentBuyToOpen},
		{Symbol: sellCallSym, Side: ds.SideSell, RatioQty: decimal.One, PositionIntent: alpaca.PositionIntentSellToOpen},
		{Symbol: buyPutSym, Side: ds.SideBuy, RatioQty: decimal.One, PositionIntent: alpaca.PositionIntentBuyToOpen},
		{Symbol: sellPutSym, Side: ds.SideSell, RatioQty: decimal.One, PositionIntent: alpaca.PositionIntentSellToOpen},
	}

	// Fetch quotes for all legs and compute natural midpoint
	client := alpaca.NewClient()
	var quoteLines []string
	naturalDebit := decimal.Zero
	allQuoted := true
	for _, leg := range orderLegs {
		snap, err := client.GetOptionSnapshot(leg.Symbol)
		if err != nil {
			quoteLines = append(quoteLines, fmt.Sprintf("  %s: error getting quote: %v", leg.Symbol, err))
			allQuoted = false
			continue
		}
		if snap.LatestQuote != nil {
			q := snap.LatestQuote
			mid := q.BidPrice.Add(q.AskPrice).DivInt(2)
			quoteLines = append(quoteLines, fmt.Sprintf("  %s (%s): bid %s ask %s mid %s",
				leg.Symbol, leg.Side, q.BidPrice, q.AskPrice, mid))
			if leg.Side == ds.SideBuy {
				naturalDebit = naturalDebit.Add(mid)
			} else {
				naturalDebit = naturalDebit.Sub(mid)
			}
		} else {
			quoteLines = append(quoteLines, fmt.Sprintf("  %s: no quote available", leg.Symbol))
			allQuoted = false
		}
	}

	// Theoretical value of box spread is (K2 - K1) per contract
	spreadWidth := strikeHigh.Sub(strikeLow)

	limitPrice := decimal.Zero
	limitStr := getStr(args, "limit_price")
	if limitStr != "" {
		limitPrice, err = decimal.ParseString(limitStr)
		if err != nil {
			return ToolCallResult{
				Content: []Content{{Type: "text", Text: fmt.Sprintf("invalid limit_price: %v", err)}},
				IsError: true,
			}
		}
	} else if allQuoted {
		// Use natural midpoint
		limitPrice = naturalDebit.QuantizeTruncate(decimal.Cent)
	} else {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "could not get quotes for all legs; please specify limit_price explicitly\n\nQuotes:\n" + join(quoteLines, "\n")}},
			IsError: true,
		}
	}

	// Warn if limit price exceeds theoretical value
	var warnings []string
	if limitPrice.Cmp(spreadWidth) > 0 {
		warnings = append(warnings, fmt.Sprintf("WARNING: limit_price %s exceeds theoretical max value %s (K2-K1)", limitPrice, spreadWidth))
	}

	tifStr := getStr(args, "time_in_force")
	timeInForce := alpaca.TimeInForceDay
	switch tifStr {
	case "gtc":
		timeInForce = alpaca.TimeInForceGTC
	case "fok":
		timeInForce = alpaca.TimeInForceFOK
	case "ioc":
		timeInForce = alpaca.TimeInForceIOC
	}

	order, err := client.CreateOrder(&alpaca.OrderRequest{
		Qty:         qty,
		Type:        alpaca.OrderTypeLimit,
		TimeInForce: timeInForce,
		LimitPrice:  limitPrice,
		OrderClass:  alpaca.OrderClassMleg,
		Legs:        orderLegs,
	})
	if err != nil {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: fmt.Sprintf("error placing box spread: %v\n\nQuotes:\n%s", err, join(quoteLines, "\n"))}},
			IsError: true,
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("box spread placed (id: %s)", order.ID))
	lines = append(lines, fmt.Sprintf("  %s %s/%s exp %s", underlying, strikeLow, strikeHigh, expiration))
	lines = append(lines, fmt.Sprintf("  qty: %s  limit: %s  tif: %s", qty, limitPrice, timeInForce))
	lines = append(lines, fmt.Sprintf("  spread width: %s (theoretical max value per contract)", spreadWidth))
	if allQuoted {
		lines = append(lines, fmt.Sprintf("  natural midpoint: %s", naturalDebit.Format(2)))
		if !naturalDebit.IsZero() {
			impliedReturn := spreadWidth.Sub(naturalDebit).Div(naturalDebit).MulInt(100)
			lines = append(lines, fmt.Sprintf("  implied return: %s%%", impliedReturn.Format(2)))
		}
	}
	lines = append(lines, "\nLegs:")
	for _, leg := range orderLegs {
		lines = append(lines, fmt.Sprintf("  %s %s ratio %s", leg.Side, leg.Symbol, leg.RatioQty))
	}
	lines = append(lines, "\nQuotes:")
	lines = append(lines, quoteLines...)
	for _, w := range warnings {
		lines = append(lines, "\n"+w)
	}

	return ToolCallResult{
		Content: []Content{{Type: "text", Text: join(lines, "\n")}},
	}
}

func join(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString(strs[0])
	for i := 1; i < len(strs); i++ {
		result.WriteString(sep + strs[i])
	}
	return result.String()
}
