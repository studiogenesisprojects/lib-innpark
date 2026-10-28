package streaming

import (
	"testing"

	"cloud.google.com/go/bigquery"
)

func TestIsInternalTable(t *testing.T) {
	internal := []string{"_collections", "_admins", "sqlite_sequence", "admins"}
	for _, name := range internal {
		if !isInternalTable(name) {
			t.Errorf("expected %q to be internal", name)
		}
	}
	business := []string{"stays", "users", "payments", "integration_requests"}
	for _, name := range business {
		if isInternalTable(name) {
			t.Errorf("expected %q to be a business table", name)
		}
	}
}

func TestIsExcludedColumn(t *testing.T) {
	exclusions := []string{"payment_methods.card_number", "token", "vehicles.qr"}

	cases := []struct {
		table, column string
		want          bool
	}{
		{"users", "password", true},              // default exclusion
		{"users", "tokenKey", true},              // default exclusion (case-insensitive)
		{"users", "name", false},                 // business column
		{"payment_methods", "card_number", true}, // table.column exclusion
		{"payments", "card_number", false},       // same column, other table
		{"any_table", "token", true},             // column exclusion (any table)
		{"vehicles", "qr", true},
		{"vehicle_qrs", "qr", false},
	}
	for _, c := range cases {
		if got := isExcludedColumn(c.table, c.column, exclusions); got != c.want {
			t.Errorf("isExcludedColumn(%q, %q) = %v, want %v", c.table, c.column, got, c.want)
		}
	}
}

func TestDeclaredTypeToBQ(t *testing.T) {
	cases := []struct {
		column, declared string
		want             bigquery.FieldType
	}{
		{"id", "TEXT", bigquery.StringFieldType},
		{"created", "TEXT", bigquery.TimestampFieldType},
		{"updated", "TEXT", bigquery.TimestampFieldType},
		{"amount", "NUMERIC", bigquery.FloatFieldType},
		{"price", "REAL", bigquery.FloatFieldType},
		{"verified", "BOOLEAN", bigquery.IntegerFieldType},
		{"counter", "INTEGER", bigquery.IntegerFieldType},
		{"options", "JSON", bigquery.StringFieldType},
		{"name", "", bigquery.StringFieldType},
		{"due_at", "DATETIME", bigquery.TimestampFieldType},
	}
	for _, c := range cases {
		if got := declaredTypeToBQ(c.column, c.declared); got != c.want {
			t.Errorf("declaredTypeToBQ(%q, %q) = %v, want %v", c.column, c.declared, got, c.want)
		}
	}
}

func TestSchemaFromSQLiteColumns(t *testing.T) {
	cols := []sqliteColumn{
		{Name: "id", Type: "TEXT"},
		{Name: "created", Type: "TEXT"},
		{Name: "updated", Type: "TEXT"},
		{Name: "plate", Type: "TEXT"},
		{Name: "amount", Type: "NUMERIC"},
		{Name: "tokenKey", Type: "TEXT"},
		{Name: "custom_secret", Type: "TEXT"},
	}
	s := schemaFromSQLiteColumns("vehicles", cols, []string{"vehicles.custom_secret"})

	want := map[string]bigquery.FieldType{
		"id":                bigquery.StringFieldType,
		"created":           bigquery.TimestampFieldType,
		"updated":           bigquery.TimestampFieldType,
		"plate":             bigquery.StringFieldType,
		"amount":            bigquery.FloatFieldType,
		fieldOperation:      bigquery.StringFieldType,
		fieldEventTimestamp: bigquery.TimestampFieldType,
	}

	if len(s) != len(want) {
		t.Fatalf("schema has %d fields, want %d: %v", len(s), len(want), s)
	}
	for _, f := range s {
		typ, ok := want[f.Name]
		if !ok {
			t.Errorf("unexpected field %q in schema", f.Name)
			continue
		}
		if f.Type != typ {
			t.Errorf("field %q has type %v, want %v", f.Name, f.Type, typ)
		}
	}
}

func TestWithEventFields(t *testing.T) {
	s := withEventFields(bigquery.Schema{
		{Name: "id", Type: bigquery.StringFieldType},
		{Name: fieldOperation, Type: bigquery.StringFieldType},
	})
	if len(s) != 3 {
		t.Fatalf("expected 3 fields (operation kept, event_timestamp added), got %d", len(s))
	}
	if !schemaHas(s, fieldEventTimestamp) {
		t.Error("expected event_timestamp to be added")
	}
}
