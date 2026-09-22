package gitscope

import (
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// The `--raw` parser's two refusals are the rules a real git cannot be made to
// break, so they are driven from here with the wire text git would have
// written. Both abort the whole changed set rather than one reading of the
// listing, because the symlink drop and pure-move detection share this parser,
// so the shape each refuses is worth pinning on its own.
func TestRawRecordsReadsTheMetadataFieldAndThePathAfterIt(t *testing.T) {
	records, err := parseRawRecords(":120000 000000 1111111111111111111111111111111111111111 0000000000000000000000000000000000000000 D\x00src/Link.cs\x00")

	want := rawRecord{
		OldMode: "120000",
		NewMode: "000000",
		Src:     "1111111111111111111111111111111111111111",
		Dst:     "0000000000000000000000000000000000000000",
		Status:  "D",
		Path:    srcpath.Path("src/Link.cs"),
	}
	if err != nil || len(records) != 1 || records[0] != want {
		t.Errorf("parseRawRecords on one record returned %v, %v, want %v and no error", records, err, want)
	}
}

func TestRawRecordsRefusesAListingThatDoesNotPair(t *testing.T) {
	records, err := parseRawRecords(":100644 120000 1111111111111111111111111111111111111111 2222222222222222222222222222222222222222 T\x00")

	if records != nil || err == nil {
		t.Fatalf("parseRawRecords on a metadata field with no path returned %v, %v, want no records and a refusal", records, err)
	}
	const wantMessage = "git diff --raw emitted 1 fields, want pairs"
	if err.Error() != wantMessage {
		t.Errorf("the refusal reads %q, want %q", err, wantMessage)
	}
}

func TestRawRecordsRefusesAMetadataFieldOfTheWrongWidth(t *testing.T) {
	records, err := parseRawRecords(":100644 120000 1111111111111111111111111111111111111111 T\x00src/Link.cs\x00")

	if records != nil || err == nil {
		t.Fatalf("parseRawRecords on a four-field record returned %v, %v, want no records and a refusal", records, err)
	}
	const wantMessage = `git diff --raw record is malformed: ":100644 120000 1111111111111111111111111111111111111111 T"`
	if err.Error() != wantMessage {
		t.Errorf("the refusal reads %q, want %q", err, wantMessage)
	}
}
