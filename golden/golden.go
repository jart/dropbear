// Package golden provides fuzzy matching of log output against golden templates.
//
// Template syntax:
//
//	...            skip any text (including newlines) until the next literal anchor
//	123.45[10]     match a number within ±10 of 123.45
//	literal text   exact character match
//
// Example template:
//
//	...=== P&L SUMMARY ===
//	...INTC: pos=0[100]   realized=83.09[50] ...
//	...total fills: 477[500]  symbols tracked: 12
package golden

import (
	"dropbear/decimal"
	"fmt"
	"strings"
	"testing"
)

const (
	red   = "\033[1;31m"
	green = "\033[32m"
	reset = "\033[0m"
)

// Check asserts that actual matches template, reporting failures to t.
// On failure, prints the error and a corrected template for copy-paste.
func Check(t testing.TB, actual, template string) {
	t.Helper()
	if err := Match(actual, template); err != nil {
		t.Errorf("golden match failed:\n%v\n\nsuggested fix (copy-paste):\n%s", err, Suggest(actual, template))
	}
}

// Match checks that actual text matches a golden template.
func Match(actual, template string) error {
	ai := 0 // position in actual
	ti := 0 // position in template

	// skip leading newline in template
	if ti < len(template) && template[ti] == '\n' {
		ti++
	}

	for ti < len(template) {
		// absorb newlines and indentation before ... (redundant since ... skips anything)
		if template[ti] == '\n' || template[ti] == ' ' || template[ti] == '\t' {
			j := ti
			for j < len(template) && (template[j] == '\n' || template[j] == ' ' || template[j] == '\t') {
				j++
			}
			if j < len(template) && strings.HasPrefix(template[j:], "...") {
				ti = j
				continue
			}
		}

		// check for skip
		if strings.HasPrefix(template[ti:], "...") {
			ti += 3
			if ti >= len(template) {
				return nil // ... at end matches everything
			}
			anchor := nextAnchor(template[ti:])
			if anchor == "" {
				return fmt.Errorf("'...' at template offset %d must be followed by literal text", ti-3)
			}
			idx := strings.Index(actual[ai:], anchor)
			if idx < 0 {
				return fmt.Errorf("anchor not found: %q\n%s",
					anchor, highlightPos(actual, ai))
			}
			ai += idx
			continue
		}

		// check for fuzzy number: -?[0-9]+(\.[0-9]+)?[tolerance]
		if val, tol, n, ok := parseFuzzy(template[ti:]); ok {
			numStr, m := scanNumber(actual[ai:])
			if m == 0 {
				return fmt.Errorf("expected number:\n%s", highlightPos(actual, ai))
			}
			got := decimal.Parse(numStr)
			diff := got.Sub(val).Abs()
			if diff.Cmp(tol) > 0 {
				return fmt.Errorf("number mismatch:\n%s\n  want %s ±%s (diff=%s)",
					highlightNumber(actual, ai, m), val, tol, diff)
			}
			ti += n
			ai += m
			continue
		}

		// bare number: match like fuzzy with tolerance 0, but give helpful error
		if tmplNum, tn := scanNumber(template[ti:]); tn > 0 {
			actNum, an := scanNumber(actual[ai:])
			if an == 0 {
				return fmt.Errorf("expected number %s:\n%s", tmplNum, highlightPos(actual, ai))
			}
			if tmplNum != actNum {
				return fmt.Errorf("number mismatch (add [N] for tolerance):\n%s\n  want %s exactly (got %s, diff=%s)",
					highlightNumber(actual, ai, an), tmplNum, actNum,
					decimal.Parse(actNum).Sub(decimal.Parse(tmplNum)).Abs())
			}
			ti += tn
			ai += an
			continue
		}

		// literal match
		if ai >= len(actual) {
			if isOnlyWhitespace(template[ti:]) {
				return nil
			}
			return fmt.Errorf("actual text ended, template continues: %q",
				template[ti:min(ti+80, len(template))])
		}
		if template[ti] != actual[ai] {
			return fmt.Errorf("literal mismatch: expected %q got %q\n%s",
				string(template[ti]), string(actual[ai]), highlightPos(actual, ai))
		}
		ti++
		ai++
	}
	return nil
}

// Suggest walks the template against actual text and returns a corrected
// template with updated tolerances. Tolerances that could be tightened
// are shown in green; tolerances that need widening are shown in red.
func Suggest(actual, template string) string {
	var out strings.Builder
	ai := 0
	ti := 0

	// skip leading newline
	if ti < len(template) && template[ti] == '\n' {
		out.WriteByte('\n')
		ti++
	}

	for ti < len(template) {
		// absorb newlines and indentation before ...
		if template[ti] == '\n' || template[ti] == ' ' || template[ti] == '\t' {
			j := ti
			for j < len(template) && (template[j] == '\n' || template[j] == ' ' || template[j] == '\t') {
				j++
			}
			if j < len(template) && strings.HasPrefix(template[j:], "...") {
				out.WriteString(template[ti:j])
				ti = j
				continue
			}
		}

		// skip
		if strings.HasPrefix(template[ti:], "...") {
			out.WriteString("...")
			ti += 3
			if ti >= len(template) {
				break
			}
			anchor := nextAnchor(template[ti:])
			if anchor == "" {
				out.WriteString(template[ti:])
				break
			}
			idx := strings.Index(actual[ai:], anchor)
			if idx < 0 {
				// can't find anchor — output rest unchanged
				out.WriteString(template[ti:])
				break
			}
			ai += idx
			continue
		}

		// fuzzy number
		if val, oldTol, n, ok := parseFuzzy(template[ti:]); ok {
			numStr, m := scanNumber(actual[ai:])
			if m == 0 {
				// can't parse number from actual — keep original
				out.WriteString(template[ti : ti+n])
				ti += n
				continue
			}
			got := decimal.Parse(numStr)
			diff := got.Sub(val).Abs()
			newTol := ceilDecimal(diff)
			valStr := val.String()
			tolStr := newTol.String()
			if newTol.Cmp(oldTol) < 0 {
				out.WriteString(fmt.Sprintf("%s[%s%s%s]", valStr, green, tolStr, reset))
			} else if newTol.Cmp(oldTol) > 0 {
				out.WriteString(fmt.Sprintf("%s[%s%s%s]", valStr, red, tolStr, reset))
			} else {
				out.WriteString(fmt.Sprintf("%s[%s]", valStr, tolStr))
			}
			ti += n
			ai += m
			continue
		}

		// bare number: suggest adding [tolerance]
		if tmplNum, tn := scanNumber(template[ti:]); tn > 0 {
			actNum, an := scanNumber(actual[ai:])
			if an > 0 {
				want := decimal.Parse(tmplNum)
				got := decimal.Parse(actNum)
				diff := got.Sub(want).Abs()
				newTol := ceilDecimal(diff)
				tolStr := newTol.String()
				if diff.IsZero() {
					out.WriteString(tmplNum)
				} else {
					out.WriteString(fmt.Sprintf("%s[%s%s%s]", tmplNum, red, tolStr, reset))
				}
				ti += tn
				ai += an
				continue
			}
		}

		// literal
		out.WriteByte(template[ti])
		ti++
		if ai < len(actual) {
			ai++
		}
	}

	return out.String()
}

// ceilDecimal rounds a non-negative decimal up to the next integer.
func ceilDecimal(d decimal.Decimal) decimal.Decimal {
	return d.QuantizeAway(decimal.One)
}

// nextAnchor returns the next run of literal text in a template,
// stopping at '...', a fuzzy number pattern, or a bare number.
// Bare numbers are excluded so they get matched positionally rather
// than becoming part of the search string (which would fail if the
// actual number differs).
func nextAnchor(tmpl string) string {
	var b strings.Builder
	i := 0
	for i < len(tmpl) {
		if strings.HasPrefix(tmpl[i:], "...") {
			break
		}
		if _, _, _, ok := parseFuzzy(tmpl[i:]); ok {
			break
		}
		// stop before bare numbers (digit or minus-then-digit)
		if isDigit(tmpl[i]) {
			break
		}
		if tmpl[i] == '-' && i+1 < len(tmpl) && isDigit(tmpl[i+1]) {
			break
		}
		b.WriteByte(tmpl[i])
		i++
	}
	return b.String()
}

// parseFuzzy tries to parse a fuzzy number like "123.45[10]" from s.
func parseFuzzy(s string) (value, tolerance decimal.Decimal, consumed int, ok bool) {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	if i >= len(s) || !isDigit(s[i]) {
		return decimal.Zero, decimal.Zero, 0, false
	}
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	if i >= len(s) || s[i] != '[' {
		return decimal.Zero, decimal.Zero, 0, false
	}
	numStr := s[:i]
	i++ // skip [
	j := i
	if j < len(s) && s[j] == '-' {
		j++
	}
	for j < len(s) && isDigit(s[j]) {
		j++
	}
	if j < len(s) && s[j] == '.' {
		j++
		for j < len(s) && isDigit(s[j]) {
			j++
		}
	}
	if j >= len(s) || s[j] != ']' {
		return decimal.Zero, decimal.Zero, 0, false
	}
	tolStr := s[i:j]
	j++ // skip ]
	return decimal.Parse(numStr), decimal.Parse(tolStr), j, true
}

// scanNumber parses a number from the start of s, returning the string and length consumed.
func scanNumber(s string) (string, int) {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	if i >= len(s) || !isDigit(s[i]) {
		return "", 0
	}
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	return s[:i], i
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isOnlyWhitespace(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
			return false
		}
	}
	return true
}

// highlightNumber shows the line containing pos with n characters highlighted in red.
func highlightNumber(s string, pos, n int) string {
	lineStart := strings.LastIndex(s[:pos], "\n") + 1
	lineEnd := strings.Index(s[pos:], "\n")
	if lineEnd < 0 {
		lineEnd = len(s)
	} else {
		lineEnd += pos
	}
	line := s[lineStart:lineEnd]
	offset := pos - lineStart
	return fmt.Sprintf("  %s%s%s%s%s",
		line[:offset], red, line[offset:offset+n], reset, line[offset+n:])
}

// highlightPos shows the line containing pos with a red caret underneath.
func highlightPos(s string, pos int) string {
	if pos >= len(s) {
		return "  <end of text>"
	}
	lineStart := strings.LastIndex(s[:pos], "\n") + 1
	lineEnd := strings.Index(s[pos:], "\n")
	if lineEnd < 0 {
		lineEnd = len(s)
	} else {
		lineEnd += pos
	}
	line := s[lineStart:lineEnd]
	offset := pos - lineStart
	return fmt.Sprintf("  %s\n  %s%s^%s", line, strings.Repeat(" ", offset), red, reset)
}
