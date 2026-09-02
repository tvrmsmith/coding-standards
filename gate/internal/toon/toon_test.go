package toon_test

import (
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/toon"
)

func TestEncode_SingleStringField(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "status", Value: "fail"},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "status: fail\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_SingleIntField(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "changed_methods", Value: 4},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "changed_methods: 4\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_MultipleFieldsPreserveOrder(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "status", Value: "fail"},
		{Key: "changed_methods", Value: 4},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "status: fail\nchanged_methods: 4\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_NilFieldRendersNullToken(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "reason", Value: nil},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "reason: null\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_StringFieldContainingColonIsQuoted(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "message", Value: "no diff base: tried origin/HEAD"},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "message: \"no diff base: tried origin/HEAD\"\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncode_NestedScalarContainingColonIsQuoted pins that colon
// quoting applies in nested-object position too, not only at the
// document's top level: the injected error-document contract depends
// on message being quoted one indent level in.
func TestEncode_NestedScalarContainingColonIsQuoted(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "error", Value: &toon.Doc{Fields: []toon.Field{
			{Key: "message", Value: "no diff base: tried origin/HEAD"},
		}}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "error:\n  message: \"no diff base: tried origin/HEAD\"\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncode_StringWithCommaButNoColonStaysBare pins the other
// direction of the colon rule: a comma is not a §7.2 trigger under the
// pipe delimiter, so a message with a comma and no colon must render
// unquoted.
func TestEncode_StringWithCommaButNoColonStaysBare(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "message", Value: "CRAP requires a coverage report, none found"},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "message: CRAP requires a coverage report, none found\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncode_StringQuotingTriggers is the full spec §7.2 "Encoders MUST
// quote a string value if any of the following is true" trigger set,
// one case per condition. The gate hands this encoder arbitrary file
// paths, C# method names (including generic ones), and free-text error
// messages, so every one of these can fire in production; pinning them
// here means a regression shows up as a failing test rather than a
// latent bug a golden happens not to exercise.
func TestEncode_StringQuotingTriggers(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string // the quoted token, or the bare value if not quoted
	}{
		{"empty string", "", `""`},
		{"leading whitespace", " x", `" x"`},
		{"trailing whitespace", "x ", `"x "`},
		{"reserved literal true", "true", `"true"`},
		{"reserved literal false", "false", `"false"`},
		{"reserved literal null", "null", `"null"`},
		{"numeric-like integer", "42", `"42"`},
		{"numeric-like signed decimal", "-3.14", `"-3.14"`},
		{"numeric-like leading zero", "05", `"05"`},
		{"numeric-like exponent", "1e-6", `"1e-6"`},
		{"numeric-like unsigned exponent", "1e6", `"1e6"`},
		{"contains colon", "a:b", `"a:b"`},
		{"contains double quote", `a"b`, `"a\"b"`},
		{"contains backslash", `a\b`, `"a\\b"`},
		{"contains active delimiter", "a|b", `"a|b"`},
		{"contains open bracket", "a[b", `"a[b"`},
		{"contains close bracket", "a]b", `"a]b"`},
		{"contains open brace", "a{b", `"a{b"`},
		{"contains close brace", "a}b", `"a}b"`},
		{"contains control character", "a\x01b", "\"a\\u0001b\""},
		{"contains newline", "a\nb", `"a\nb"`},
		{"leading hyphen", "-x", `"-x"`},
		{"bare hyphen", "-", `"-"`},
		{"leading number sign", "#x", `"#x"`},
		{"bare number sign", "#", `"#"`},
		{"ordinary word stays bare", "measured", "measured"},
		{"metric name stays bare", "merge-base", "merge-base"},
		{"file path stays bare", "src/A.cs", "src/A.cs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := toon.Doc{Fields: []toon.Field{{Key: "value", Value: tt.value}}}

			got, err := doc.Encode()

			if err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
			want := "value: " + tt.want + "\n"
			if string(got) != want {
				t.Errorf("Encode() = %q, want %q", got, want)
			}
		})
	}
}

func TestEncode_EmptyStringSliceFieldRendersEmptyBrackets(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "skipped_paths", Value: []string{}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "skipped_paths: []\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_NonEmptyStringSliceFieldRendersInlineArray(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "skipped_paths", Value: []string{"src/Legacy.cs", "src/Other.cs"}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "skipped_paths[2|]: src/Legacy.cs|src/Other.cs\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_EmptyTableFieldRendersEmptyBrackets(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{{Name: "file"}, {Name: "complexity"}},
		}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "crap: []\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_TableFieldRendersHeaderAndIndentedRows(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{{Name: "file"}, {Name: "complexity"}},
			Rows: [][]any{
				{"src/Ordering/Pricing.cs", 34},
				{"src/Ordering/Order.cs", 1},
			},
		}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "crap[2|]{file|complexity}:\n" +
		"  src/Ordering/Pricing.cs|34\n" +
		"  src/Ordering/Order.cs|1\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_TableRowWithWrongArityReturnsError(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{{Name: "file"}, {Name: "complexity"}},
			Rows: [][]any{
				{"src/Ordering/Pricing.cs"},
			},
		}},
	}}

	_, err := doc.Encode()

	if err == nil {
		t.Fatal("Encode() error = nil, want non-nil for a row whose length does not match Columns")
	}
}

func TestEncode_TableCellOfUnsupportedTypeReturnsError(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{{Name: "file"}},
			Rows: [][]any{
				{true},
			},
		}},
	}}

	_, err := doc.Encode()

	if err == nil {
		t.Fatal("Encode() error = nil, want non-nil for a cell of an unsupported type")
	}
}

func TestEncode_FloatFieldRendersCanonicalDecimal(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "ratio", Value: 1.5},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "ratio: 1.5\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_IntegralFloatFieldDropsFraction(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "ratio", Value: 1.0},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "ratio: 1\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_TableFloatCellRoundsToColumnPrecisionThenStripsTrailingZeros(t *testing.T) {
	// Precision is a rounding rule, not a padding rule: 0.5504 rounds to
	// 0.550 at precision 3 and then renders canonically as 0.55.
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{{Name: "coverage", Precision: 3}},
			Rows: [][]any{
				{0.5504},
			},
		}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "crap[1|]{coverage}:\n  0.55\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_TableFloatCellIntegralAfterRoundingDropsFraction(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{{Name: "coverage", Precision: 3}},
			Rows: [][]any{
				{1.0},
			},
		}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "crap[1|]{coverage}:\n  1\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestEncode_TableFloatCellRoundsHalfUpNotHalfToEven(t *testing.T) {
	// 0.125 is exactly representable in binary (1/8), so this is a genuine
	// decimal tie at precision 2, not a binary-rounding artifact. Go's
	// strconv formatting (round-half-to-even) would render 0.12; the
	// contract requires round-half-up, 0.13.
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{{Name: "score", Precision: 2}},
			Rows: [][]any{
				{0.125},
			},
		}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "crap[1|]{score}:\n  0.13\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncode_ADR0005WorkedExample is the fixed-bytes contract from
// ADR 0005 ("The machine document is the only output"), as amended
// after the spec §2 ruling on Column.Precision (round, then render
// canonically; no padded trailing zeros). Every byte here comes from
// that document, not from re-deriving the expected value in this test.
func TestEncode_ADR0005WorkedExample(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "status", Value: "fail"},
		{Key: "tool", Value: "metric-gate/0.1.0"},
		{Key: "spec", Value: "toon/4.1.1"},
		{Key: "scope", Value: "merge-base"},
		{Key: "base", Value: "origin/main@9f3c110"},
		{Key: "changed_methods", Value: 4},
		{Key: "touched_lines_outside_spans", Value: 1},
		{Key: "skipped_paths", Value: []string{}},
		{Key: "metrics", Value: &toon.Table{
			Columns: []toon.Column{{Name: "name"}, {Name: "threshold"}, {Name: "measured"}, {Name: "failed"}},
			Rows: [][]any{
				{"crap", 30, 4, 2},
			},
		}},
		{Key: "crap", Value: &toon.Table{
			Columns: []toon.Column{
				{Name: "file"}, {Name: "start"}, {Name: "end"}, {Name: "name"},
				{Name: "complexity"}, {Name: "coverage", Precision: 3}, {Name: "score", Precision: 2},
				{Name: "state"}, {Name: "action"}, {Name: "target_coverage", Precision: 3}, {Name: "reason"},
			},
			Rows: [][]any{
				{"src/Ordering/Pricing.cs", 18, 71, "Pricing.Quote", 34, 0.55, 139.34, "measured", "split_method", nil, nil},
				{"src/Ordering/OrderService.cs", 41, 58, "OrderService.PlaceAsync", 9, 0.1, 68.05, "measured", "raise_coverage", 0.363, nil},
				{"src/Ordering/OrderService.cs", 60, 64, "OrderService.Cancel", 3, 2.0 / 3.0, 3.33, "measured", "none", nil, nil},
				{"src/Ordering/Order.cs", 14, 14, "Order.get_Id", 1, 1.0, 1.0, "structural_na", "none", nil, nil},
			},
		}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "status: fail\n" +
		"tool: metric-gate/0.1.0\n" +
		"spec: toon/4.1.1\n" +
		"scope: merge-base\n" +
		"base: origin/main@9f3c110\n" +
		"changed_methods: 4\n" +
		"touched_lines_outside_spans: 1\n" +
		"skipped_paths: []\n" +
		"metrics[1|]{name|threshold|measured|failed}:\n" +
		"  crap|30|4|2\n" +
		"crap[4|]{file|start|end|name|complexity|coverage|score|state|action|target_coverage|reason}:\n" +
		"  src/Ordering/Pricing.cs|18|71|Pricing.Quote|34|0.55|139.34|measured|split_method|null|null\n" +
		"  src/Ordering/OrderService.cs|41|58|OrderService.PlaceAsync|9|0.1|68.05|measured|raise_coverage|0.363|null\n" +
		"  src/Ordering/OrderService.cs|60|64|OrderService.Cancel|3|0.667|3.33|measured|none|null|null\n" +
		"  src/Ordering/Order.cs|14|14|Order.get_Id|1|1|1|structural_na|none|null|null\n"
	if string(got) != want {
		t.Errorf("Encode() =\n%s\nwant\n%s", got, want)
	}
}

func TestEncode_NestedDocFieldIndentsTwoSpacesDeeper(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "status", Value: "error"},
		{Key: "error", Value: &toon.Doc{Fields: []toon.Field{
			{Key: "code", Value: "no_diff_base"},
			{Key: "message", Value: "no diff base"},
		}}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "status: error\n" +
		"error:\n" +
		"  code: no_diff_base\n" +
		"  message: no diff base\n"
	if string(got) != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncode_ErrorDocumentWorkedExample is the injected fixed-bytes
// contract for a nested *Doc field (the metric-gate error path).
func TestEncode_ErrorDocumentWorkedExample(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "status", Value: "error"},
		{Key: "tool", Value: "metric-gate/0.1.0"},
		{Key: "spec", Value: "toon/4.1.1"},
		{Key: "scope", Value: "merge-base"},
		{Key: "base", Value: nil},
		{Key: "changed_methods", Value: 0},
		{Key: "touched_lines_outside_spans", Value: 0},
		{Key: "skipped_paths", Value: []string{}},
		{Key: "error", Value: &toon.Doc{Fields: []toon.Field{
			{Key: "code", Value: "no_diff_base"},
			{Key: "message", Value: "no diff base: tried origin/HEAD, origin/main, origin/master, main, master"},
		}}},
	}}

	got, err := doc.Encode()

	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	want := "status: error\n" +
		"tool: metric-gate/0.1.0\n" +
		"spec: toon/4.1.1\n" +
		"scope: merge-base\n" +
		"base: null\n" +
		"changed_methods: 0\n" +
		"touched_lines_outside_spans: 0\n" +
		"skipped_paths: []\n" +
		"error:\n" +
		"  code: no_diff_base\n" +
		"  message: \"no diff base: tried origin/HEAD, origin/main, origin/master, main, master\"\n"
	if string(got) != want {
		t.Errorf("Encode() =\n%s\nwant\n%s", got, want)
	}
}

func TestEncode_UnsupportedFieldTypeReturnsError(t *testing.T) {
	doc := toon.Doc{Fields: []toon.Field{
		{Key: "bad", Value: true},
	}}

	_, err := doc.Encode()

	if err == nil {
		t.Fatal("Encode() error = nil, want non-nil for unsupported field type")
	}
}
