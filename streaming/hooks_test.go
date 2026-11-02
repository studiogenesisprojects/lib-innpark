package streaming

import (
	"testing"

	"cloud.google.com/go/bigquery"
	"github.com/pocketbase/dbx"
	_ "modernc.org/sqlite"
)

func TestStoredRowReadsEveryColumnAsStored(t *testing.T) {
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, q := range []string{
		`CREATE TABLE stays (id TEXT PRIMARY KEY, start_pending BOOLEAN, amount NUMERIC, options JSON, closed_at TEXT, updated TEXT)`,
		`INSERT INTO stays VALUES ('s1', 1, 12.5, '{"a":1}', NULL, '2026-11-02 10:00:00.123Z')`,
	} {
		if _, err := db.NewQuery(q).Execute(); err != nil {
			t.Fatal(err)
		}
	}

	row, err := storedRow(db, "stays", "s1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"id": "s1", "start_pending": "1", "amount": "12.5", "options": `{"a":1}`,
		"closed_at": nil, "updated": "2026-11-02 10:00:00.123Z",
	}
	if len(row) != len(want) {
		t.Fatalf("got %d columns, want %d: %v", len(row), len(want), row)
	}
	for k, v := range want {
		if row[k] != v {
			t.Errorf("%s = %#v, want %#v", k, row[k], v)
		}
	}

	// The stored row must survive the payload pipeline with the right types.
	schema := bigquery.Schema{
		{Name: "id", Type: bigquery.StringFieldType},
		{Name: "start_pending", Type: bigquery.IntegerFieldType},
		{Name: "amount", Type: bigquery.FloatFieldType},
		{Name: "updated", Type: bigquery.TimestampFieldType},
	}
	data := filterSchemaFields(schema, row)
	coerceTypes(schema, data)
	sanitizeByType(schema, data)
	if data["start_pending"] != int64(1) || data["amount"] != 12.5 || data["updated"] != "2026-11-02 10:00:00.123Z" {
		t.Errorf("unexpected payload: %#v", data)
	}

	missing, err := storedRow(db, "stays", "nope")
	if err != nil || missing != nil {
		t.Errorf("missing row: got %v, %v; want nil, nil", missing, err)
	}
}

func TestIntegerColumnsRoundInsteadOfTruncating(t *testing.T) {
	schema := bigquery.Schema{
		{Name: "a", Type: bigquery.IntegerFieldType},
		{Name: "b", Type: bigquery.IntegerFieldType},
	}
	data := map[string]interface{}{"a": 2.6, "b": "2.6"}
	coerceTypes(schema, data)
	sanitizeByType(schema, data)
	if data["a"] != int64(3) || data["b"] != int64(3) {
		t.Errorf("got %#v, want a=3 b=3", data)
	}
}
