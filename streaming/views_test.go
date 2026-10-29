package streaming

import (
	"strings"
	"testing"
)

func TestDedupViewSQL(t *testing.T) {
	sql := DedupViewSQL("innpark", "prod_onstreet", "stays_events", "stays")

	for _, want := range []string{
		"CREATE VIEW `innpark.prod_onstreet.stays`",
		"FROM `innpark.prod_onstreet.stays_events`",
		"ROW_NUMBER() OVER (PARTITION BY id ORDER BY updated DESC, event_timestamp DESC)",
		"SELECT * EXCEPT(operation, event_timestamp, _rk)",
		"WHERE _rk = 1 AND COALESCE(operation, '') != 'delete'",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("dedup view SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestWatermarkViewSQL(t *testing.T) {
	sql := WatermarkViewSQL("innpark", "prod_onstreet", []string{"stays", "zones"})

	for _, want := range []string{
		"CREATE VIEW `innpark.prod_onstreet.etl_watermark`",
		"SELECT 'stays' AS table_name, MAX(updated) AS last_updated, COUNT(*) AS row_count FROM `innpark.prod_onstreet.stays`",
		"UNION ALL",
		"SELECT 'zones' AS table_name",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("watermark view SQL missing %q:\n%s", want, sql)
		}
	}
}
func TestDedupViewSQLWithNulledColumns(t *testing.T) {
	sql := DedupViewSQL("innpark", "prod_onstreet", "users_events", "users",
		NulledColumn{Name: "avatar", Type: "STRING"},
		NulledColumn{Name: "passwordHash", Type: "STRING"},
	)
	for _, want := range []string{
		"SELECT * EXCEPT(operation, event_timestamp, _rk),\n  CAST(NULL AS STRING) AS avatar,\n  CAST(NULL AS STRING) AS passwordHash FROM (",
		"FROM `innpark.prod_onstreet.users_events`",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("view SQL missing %q:\n%s", want, sql)
		}
	}
}
