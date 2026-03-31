// schwablog pretty-prints raw schwab websocket log files.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: schwablog FILE\n")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		processLine(line)
	}
}

func processLine(line string) {
	// parse: TIMESTAMP got message type N: JSON
	idx := strings.Index(line, " got message type ")
	if idx < 0 {
		return
	}
	timestamp := line[:idx]
	rest := line[idx+len(" got message type "):]
	colonIdx := strings.Index(rest, ": ")
	if colonIdx < 0 {
		return
	}
	body := rest[colonIdx+2:]

	// skip heartbeats
	if strings.Contains(body, `"heartbeat"`) {
		return
	}

	// parse the outer envelope
	var envelope struct {
		Data []struct {
			Service   string            `json:"service"`
			Timestamp int64             `json:"timestamp"`
			Command   string            `json:"command"`
			Content   []json.RawMessage `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		fmt.Printf("// %s\n// parse error: %v\n%s\n\n", timestamp, err, body)
		return
	}

	for _, data := range envelope.Data {
		for _, content := range data.Content {
			// parse content as map to find field "3" which is embedded JSON
			var m map[string]json.RawMessage
			if err := json.Unmarshal(content, &m); err != nil {
				fmt.Printf("// %s\n", timestamp)
				prettyPrint(content)
				fmt.Println()
				continue
			}

			// unescape field "3" from string to actual JSON
			if raw, ok := m["3"]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil {
					m["3"] = json.RawMessage(s)
				}
			}

			// print event type if available
			var eventType string
			if raw, ok := m["2"]; ok {
				json.Unmarshal(raw, &eventType)
			}
			if eventType == "SUBSCRIBED" {
				continue
			}

			rebuilt, _ := json.Marshal(m)
			if eventType != "" {
				fmt.Printf("// %s %s\n", timestamp, eventType)
			} else {
				fmt.Printf("// %s\n", timestamp)
			}
			prettyPrint(rebuilt)
			fmt.Println()
		}
	}
}

func prettyPrint(data []byte) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		os.Stdout.Write(data)
		fmt.Println()
		return
	}
	v = simplifyProtoDecimals(v)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// simplifyProtoDecimals walks the JSON tree and collapses Schwab's
// protobuf decimal objects like {"lo":"650000","signScale":"12"} into
// readable decimal strings like "0.65".
func simplifyProtoDecimals(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if d, ok := tryDecimal(v); ok {
			return d
		}
		if ts, ok := tryTimestamp(v); ok {
			return ts
		}
		for k, val := range v {
			v[k] = simplifyProtoDecimals(val)
		}
		return v
	case []any:
		for i, val := range v {
			v[i] = simplifyProtoDecimals(val)
		}
		return v
	default:
		return v
	}
}

func tryTimestamp(m map[string]any) (string, bool) {
	if len(m) == 1 {
		if s, ok := m["DateTimeString"].(string); ok {
			return s, true
		}
	}
	return "", false
}

func tryDecimal(m map[string]any) (string, bool) {
	_, hasScale := m["signScale"]
	loRaw, hasLo := m["lo"]
	if !hasLo && !hasScale {
		return "", false
	}
	if !hasLo {
		return "0", true
	}
	// lo can be string or float64 from JSON
	var lo int64
	switch v := loRaw.(type) {
	case string:
		var neg bool
		i := 0
		if i < len(v) && v[i] == '-' {
			neg = true
			i++
		}
		for i < len(v) {
			c := v[i]
			if c < '0' || c > '9' {
				return "", false
			}
			lo = lo*10 + int64(c-'0')
			i++
		}
		if neg {
			lo = -lo
		}
	case float64:
		lo = int64(v)
	default:
		return "", false
	}
	if !hasScale {
		// plain integer (e.g. AskSize)
		return fmt.Sprintf("%d", lo), true
	}
	// lo is pre-scaled by 10^6
	neg := ""
	if lo < 0 {
		neg = "-"
		lo = -lo
	}
	whole := lo / 1_000_000
	frac := lo % 1_000_000
	if frac == 0 {
		return fmt.Sprintf("%s%d", neg, whole), true
	}
	s := fmt.Sprintf("%s%d.%06d", neg, whole, frac)
	// trim trailing zeros
	s = strings.TrimRight(s, "0")
	return s, true
}
