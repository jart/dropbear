package main

import (
	"bufio"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// isOptionSymbol returns true if the symbol looks like an OCC options symbol.
// OCC format: ROOT + YYMMDD + C/P + STRIKE (e.g., AAPL240119C00190000)
var optionSymbolRegex = regexp.MustCompile(`^[A-Z]{1,6}\d{6}[CP]\d{8}$`)

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
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
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
					Description: "Order class: simple, bracket, oco, oto",
					Enum:        []string{"simple", "bracket", "oco", "oto"},
				},
			},
			Required: []string{"symbols", "order_type"},
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
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
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
		case "notifications/initialized":
			continue
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

		encoder.Encode(resp)
	}
}

func handleToolCall(params ToolCallParams) ToolCallResult {
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
	if len(symbols) == 0 {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "no symbols specified"}},
			IsError: true,
		}
	}

	qtyStr := getStr(args, "qty")
	amtStr := getStr(args, "amt")
	if qtyStr == "" && amtStr == "" {
		return ToolCallResult{
			Content: []Content{{Type: "text", Text: "must specify qty or amt"}},
			IsError: true,
		}
	}

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
	}

	client := alpaca.NewClient()
	var results []string

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
