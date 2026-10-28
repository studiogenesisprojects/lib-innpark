package streaming

import "fmt"

// DedupViewSQL returns the DDL for the consumer view of a table: one row per
// id (latest version wins), with deleted rows hidden. The view exposes
// exactly the business columns of the events table (the event envelope is
// hidden), so existing consumers keep working unchanged.
func DedupViewSQL(project, dataset, eventsTable, viewName string) string {
	events := fmt.Sprintf("`%s.%s.%s`", project, dataset, eventsTable)
	view := fmt.Sprintf("`%s.%s.%s`", project, dataset, viewName)

	return fmt.Sprintf(
		`CREATE VIEW %s AS
SELECT * EXCEPT(%s, %s, _rk) FROM (
  SELECT *,
    ROW_NUMBER() OVER (PARTITION BY id ORDER BY updated DESC, %s DESC) AS _rk
  FROM %s
)
WHERE _rk = 1 AND COALESCE(%s, '') != 'delete'`,
		view,
		fieldOperation, fieldEventTimestamp,
		fieldEventTimestamp,
		events,
		fieldOperation,
	)
}

// WatermarkViewSQL returns the DDL for the freshness view that replaces the
// legacy tables.get() metadata reads: one row per table with the latest
// event timestamp and the current row count.
func WatermarkViewSQL(project, dataset string, viewNames []string) string {
	union := ""
	for i, name := range viewNames {
		if i > 0 {
			union += "\nUNION ALL\n"
		}
		union += fmt.Sprintf(
			"SELECT '%s' AS table_name, MAX(updated) AS last_updated, COUNT(*) AS row_count FROM `%s.%s.%s`",
			name, project, dataset, name,
		)
	}
	return fmt.Sprintf("CREATE VIEW `%s.%s.%s` AS\n%s", project, dataset, "etl_watermark", union)
}
