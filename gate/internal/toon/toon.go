// Package toon is an in-house encoder for the TOON document format
// (https://toonformat.dev/), spec version 4.1.1. It renders the document
// model metric-gate emits on stdout per ADR 0005: a fixed pipe delimiter,
// tabular arrays for uniform rows, and no decoding — this gate never reads
// TOON back in.
package toon

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SpecVersion is the TOON specification version this encoder targets.
const SpecVersion = "4.1.1"

// Doc is a TOON document, or a nested object when it is the Value of a
// Field: an ordered list of fields.
type Doc struct {
	Fields []Field
}

// Field is one key of a Doc. Value must be a string, int, float64,
// nil, []string, *Table, or *Doc; Encode returns an error for any other
// type.
type Field struct {
	Key   string
	Value any
}

// Table is a uniform tabular array (spec §9.3): a fixed column list
// followed by one row per element.
type Table struct {
	Columns []Column
	Rows    [][]any
}

// Column names one field of a Table's rows. Precision is the number of
// decimal places to render a float64 cell in this column at; it is
// ignored for cells of any other type.
type Column struct {
	Name      string
	Precision int
}

// Encode renders d as a TOON document.
func (d Doc) Encode() ([]byte, error) {
	return encodeDoc(d, 0)
}

// encodeDoc renders d's fields at the given indent level (each level is
// two spaces).
func encodeDoc(d Doc, indent int) ([]byte, error) {
	var buf []byte
	for _, f := range d.Fields {
		line, err := encodeField(f, indent)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Key, err)
		}
		buf = append(buf, line...)
	}
	return buf, nil
}

// indentPrefix returns the two-space-per-level indent for indent.
func indentPrefix(indent int) string {
	return strings.Repeat("  ", indent)
}

// encodeField renders one field's line(s) at the given indent level,
// including its trailing newline(s). A nested *Doc field renders as its
// key, a colon, and a newline, with its own fields one indent level
// deeper; nesting is unbounded in the type though this project only
// ever uses one level.
func encodeField(f Field, indent int) (string, error) {
	prefix := indentPrefix(indent)
	switch val := f.Value.(type) {
	case []string:
		line, err := encodeStringSliceField(f.Key, val)
		return prefix + line, err
	case *Table:
		line, err := encodeTableField(f.Key, val, indent)
		return prefix + line, err
	case *Doc:
		body, err := encodeDoc(*val, indent+1)
		if err != nil {
			return "", err
		}
		return prefix + f.Key + ":\n" + string(body), nil
	}
	token, err := scalarToken(f.Value)
	if err != nil {
		return "", err
	}
	return prefix + f.Key + ": " + token + "\n", nil
}

// encodeTableField renders a *Table field at the given indent level. A
// table with zero rows has no uniform shape to declare and MUST be
// emitted as "key: []", the same empty-array form as an empty []string
// field (spec §9.1); ADR 0005 makes this rule explicit for the gate's
// own tables. A non-empty table is the tabular header form
// "key[N<delim?>]{field1<delim>...}:" (spec §6), followed by one row
// per element one indent level deeper (spec §9.3), each row's cells
// joined by the active delimiter.
func encodeTableField(key string, t *Table, indent int) (string, error) {
	if len(t.Rows) == 0 {
		return key + ": []\n", nil
	}
	names := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		names[i] = c.Name
	}
	header := fmt.Sprintf("%s[%d%c]{%s}:\n", key, len(t.Rows), delimiter, strings.Join(names, string(delimiter)))

	rowPrefix := indentPrefix(indent + 1)
	var rows strings.Builder
	for _, row := range t.Rows {
		if len(row) != len(t.Columns) {
			return "", fmt.Errorf("row has %d cells, want %d to match Columns", len(row), len(t.Columns))
		}
		cells := make([]string, len(row))
		for i, cell := range row {
			token, err := cellToken(cell, t.Columns[i])
			if err != nil {
				return "", fmt.Errorf("column %q: %w", t.Columns[i].Name, err)
			}
			cells[i] = token
		}
		rows.WriteString(rowPrefix)
		rows.WriteString(strings.Join(cells, string(delimiter)))
		rows.WriteString("\n")
	}
	return header + rows.String(), nil
}

// encodeStringSliceField renders a []string field as a TOON array of
// primitives (spec §9.1). An empty slice MUST be emitted as "key: []"
// rather than the legacy "key[0<delim?>]:" header ("encoders MUST NOT
// emit" that form, §9.1). A non-empty slice is the inline form
// "key[N<delim?>]: v1<delim>v2<delim>...", with N the element count and
// each element quoted per §7.2 like any other primitive.
func encodeStringSliceField(key string, items []string) (string, error) {
	if len(items) == 0 {
		return key + ": []\n", nil
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = quoteString(item)
	}
	header := fmt.Sprintf("%s[%d%c]: ", key, len(items), delimiter)
	return header + strings.Join(quoted, string(delimiter)) + "\n", nil
}

// delimiter is the pipe character this document always encodes with
// (ADR 0005), both as the document delimiter for object field values and
// the active delimiter for table headers, rows, and inline arrays
// (spec §11.1).
const delimiter = '|'

// quoteString renders a string field or cell, quoting and escaping it
// when the spec requires disambiguation from other tokens.
func quoteString(s string) string {
	if !needsQuoting(s) {
		return s
	}
	return `"` + escapeString(s) + `"`
}

// numericLike matches spec §7.2's numeric-like test:
// "/^[+-]?[0-9]+(?:\.[0-9]+)?(?:e[+-]?[0-9]+)?$/i" (ASCII digits only).
var numericLike = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// needsQuoting reports whether s falls under any of the spec §7.2
// "Encoders MUST quote a string value if any of the following is true"
// conditions:
//   - "It is empty (\"\")."
//   - "It has leading or trailing whitespace (U+0020 or U+0009)."
//   - "It equals true, false, or null (case-sensitive)."
//   - "It is numeric-like" (matches numericLike above).
//   - "It contains a colon (:), double quote ("), or backslash (\\)."
//   - "It contains brackets or braces ([, ], {, })."
//   - "It contains control characters in U+0000 through U+001F."
//   - "It contains the relevant delimiter" (the pipe delimiter, §11.1).
//   - "It equals \"-\" or starts with \"-\" (any hyphen at position 0)."
//   - "It equals \"#\" or starts with \"#\" (any number sign at position 0)."
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	if s[0] == ' ' || s[0] == '\t' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t' {
		return true
	}
	if s == "true" || s == "false" || s == "null" {
		return true
	}
	if s[0] == '-' || s[0] == '#' {
		return true
	}
	if numericLike.MatchString(s) {
		return true
	}
	for _, r := range s {
		switch {
		case r == delimiter || r == '"' || r == ':' || r == '\\':
			return true
		case r == '[' || r == ']' || r == '{' || r == '}':
			return true
		case r <= 0x1F:
			return true
		}
	}
	return false
}

// escapeString applies the spec §7.1 escape sequences this encoder
// produces: backslash -> "\\", double quote -> "\"", LF -> "\n",
// CR -> "\r", HTAB -> "\t", and any other C0 control character
// (U+0000-U+001F) -> "\uXXXX" with lowercase hex.
func escapeString(s string) string {
	var buf []byte
	for _, r := range s {
		switch r {
		case '\\':
			buf = append(buf, '\\', '\\')
		case '"':
			buf = append(buf, '\\', '"')
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		default:
			if r <= 0x1F {
				buf = append(buf, fmt.Sprintf(`\u%04x`, r)...)
			} else {
				buf = append(buf, string(r)...)
			}
		}
	}
	return string(buf)
}

// formatCanonicalFloat renders a top-level float64 field in the spec §2
// canonical decimal form used for values with no Column to carry a
// Precision: no exponent, no fractional trailing zeros ("1.5000 MUST be
// rendered as 1.5"), and the fraction dropped entirely when it is zero
// ("If the fractional part is zero after normalization, emit as an
// integer"). strconv's 'f'/-1 verb already produces the shortest
// round-tripping decimal, which is this canonical form for any value in
// the range metric-gate's fields fall in.
func formatCanonicalFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// scalarToken renders a single scalar cell/field value. nil MUST be
// emitted as the lowercase literal "null" (spec §2, Data Model).
func scalarToken(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "null", nil
	case string:
		return quoteString(val), nil
	case int:
		return strconv.Itoa(val), nil
	case float64:
		return formatCanonicalFloat(val), nil
	default:
		return "", fmt.Errorf("unsupported value type %T", v)
	}
}

// cellToken renders one table cell, rounding a float64 cell to
// col.Precision before rendering it. Precision is a rounding rule, not
// a padding rule: the result is spec §2 canonical decimal, trailing
// zeros stripped, per metric-gate-lead's ruling on the earlier spec
// escalation (fixed-width padding, e.g. "1.000", was rejected as a
// deliberate §2 violation; rounding to Precision and then rendering
// canonically is not).
func cellToken(v any, col Column) (string, error) {
	if f, ok := v.(float64); ok {
		return formatCanonicalFloat(roundHalfUp(f, col.Precision)), nil
	}
	return scalarToken(v)
}

// roundHalfUp rounds f to precision decimal places, rounding half away
// from zero rather than Go's default round-half-to-even. It works from
// strconv's shortest round-tripping decimal representation of f, then
// rounds that decimal string, so a binary value like 0.1 (stored as
// 0.1000000000000000055511151231257827) rounds as a human reading "0.1"
// would expect rather than picking up spurious trailing binary noise.
func roundHalfUp(f float64, precision int) float64 {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	intPart, fracPart, _ := strings.Cut(s, ".")
	if len(fracPart) > precision {
		roundUp := fracPart[precision] >= '5'
		fracPart = fracPart[:precision]
		if roundUp {
			var carry bool
			fracPart, carry = incrementDigits(fracPart)
			if carry {
				intPart = incrementDigits1(intPart)
			}
		}
	}

	rounded := intPart
	if fracPart != "" {
		rounded += "." + fracPart
	}
	if neg {
		rounded = "-" + rounded
	}
	// The rounded decimal string always round-trips exactly at this
	// precision, so the reparse below cannot fail.
	result, _ := strconv.ParseFloat(rounded, 64)
	return result
}

// incrementDigits adds one to the least-significant digit of a decimal
// digit string, propagating any carry leftward. It reports whether the
// carry propagated past the most significant digit (e.g. "99" -> "00",
// true).
func incrementDigits(digits string) (string, bool) {
	b := []byte(digits)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != '9' {
			b[i]++
			return string(b), false
		}
		b[i] = '0'
	}
	return string(b), true
}

// incrementDigits1 adds one to a decimal digit string representing a
// non-negative integer, growing it by a leading "1" on overflow (e.g.
// "99" -> "100").
func incrementDigits1(digits string) string {
	b := []byte(digits)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != '9' {
			b[i]++
			return string(b)
		}
		b[i] = '0'
	}
	return "1" + string(b)
}
