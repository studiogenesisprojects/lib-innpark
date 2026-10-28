package streaming

import (
	"strings"

	"cloud.google.com/go/bigquery"
)

const (
	fieldOperation      = "operation"
	fieldEventTimestamp = "event_timestamp"
)

// defaultExcludedColumns never leave the database, regardless of options.
var defaultExcludedColumns = []string{"password", "passwordhash", "tokenkey"}

// sqliteColumn is a column of a PocketBase SQLite table (PRAGMA table_info).
type sqliteColumn struct {
	Name string `db:"name"`
	Type string `db:"type"`
}

// isInternalTable reports whether the table is internal to PocketBase and
// must never be streamed.
func isInternalTable(name string) bool {
	return strings.HasPrefix(name, "_") ||
		strings.HasPrefix(name, "sqlite_") ||
		name == "admins"
}

// isExcludedColumn reports whether the column is excluded from streaming.
// Exclusions match case-insensitively as "column" (any table) or "table.column".
func isExcludedColumn(table, column string, exclusions []string) bool {
	col := strings.ToLower(column)
	full := strings.ToLower(table + "." + column)

	for _, e := range append(defaultExcludedColumns, exclusions...) {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if e == col || e == full {
			return true
		}
	}
	return false
}

// isEventField reports whether the column belongs to the event envelope
// added by this package on top of the business columns.
func isEventField(name string) bool {
	return name == fieldOperation || name == fieldEventTimestamp
}

// eventFields are appended to every events table schema.
func eventFields() bigquery.Schema {
	return bigquery.Schema{
		{Name: fieldOperation, Type: bigquery.StringFieldType},
		{Name: fieldEventTimestamp, Type: bigquery.TimestampFieldType},
	}
}

// declaredTypeToBQ maps a SQLite declared column type to a BigQuery type.
// It intentionally mirrors the historical Python exporter (export_all.py):
// ints/bools as INTEGER, real/numeric as FLOAT, created/updated as TIMESTAMP
// and everything else as STRING, so the events tables stay compatible with
// the tables the consumers already know.
func declaredTypeToBQ(column, declared string) bigquery.FieldType {
	if column == "created" || column == "updated" {
		return bigquery.TimestampFieldType
	}

	t := strings.ToUpper(strings.TrimSpace(declared))
	switch {
	case strings.HasPrefix(t, "INT"), strings.HasPrefix(t, "BOOL"):
		return bigquery.IntegerFieldType
	case strings.HasPrefix(t, "REAL"), strings.HasPrefix(t, "FLO"),
		strings.HasPrefix(t, "DOU"), strings.HasPrefix(t, "NUM"):
		return bigquery.FloatFieldType
	case strings.HasPrefix(t, "DATETIME"), strings.HasPrefix(t, "DATE"),
		strings.HasPrefix(t, "TIMESTAMP"):
		return bigquery.TimestampFieldType
	default:
		return bigquery.StringFieldType
	}
}

// schemaFromSQLiteColumns builds the BigQuery schema of an events table from
// the SQLite columns of the source table, excluding sensitive columns and
// appending the event envelope. It is the fallback used when there is no
// existing BigQuery table to copy the schema from.
func schemaFromSQLiteColumns(table string, cols []sqliteColumn, exclusions []string) bigquery.Schema {
	s := bigquery.Schema{}
	for _, c := range cols {
		if isExcludedColumn(table, c.Name, exclusions) {
			continue
		}
		s = append(s, &bigquery.FieldSchema{
			Name: c.Name,
			Type: declaredTypeToBQ(c.Name, c.Type),
		})
	}
	return append(s, eventFields()...)
}
