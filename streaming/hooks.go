package streaming

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// streamLog writes streaming logs to stdout directly. PocketBase's app.Logger()
// switches to its internal logs store after bootstrap, which hides messages
// from Cloud Logging on Cloud Run (they only print with --dev). Streaming must
// stay observable at all times.
var streamLog = slog.New(slog.NewTextHandler(os.Stdout, nil))

// RegisterHooks streams every model change (create, update and delete) of
// the tables enabled for streaming. Call it once at startup, after
// InitPublisher.
func RegisterHooks(app core.App) {
	app.OnModelAfterCreate().Add(func(e *core.ModelEvent) error {
		streamModelChange(app, e, "create")
		return nil
	})
	app.OnModelAfterUpdate().Add(func(e *core.ModelEvent) error {
		streamModelChange(app, e, "update")
		return nil
	})
	app.OnModelAfterDelete().Add(func(e *core.ModelEvent) error {
		streamModelChange(app, e, "delete")
		return nil
	})
}

// streamModelChange publishes the change asynchronously so the database
// transaction is never blocked. The payload is built before the goroutine
// because the model may change afterwards.
//
// It must never interfere with the request: every failure is only logged and
// panics are recovered.
func streamModelChange(app core.App, e *core.ModelEvent, operation string) {
	defer func() {
		if r := recover(); r != nil {
			streamLog.Warn("streaming hook panic recovered",
				"operation", operation, "panic", r)
		}
	}()

	p := GetPublisher()
	if p == nil || e == nil || e.Model == nil {
		return
	}

	table := e.Model.TableName()
	if !p.ShouldStream(table) {
		return
	}

	// Captured here, not in the goroutine: two saves of the same row within
	// the same millisecond must keep their order in the events table.
	eventTime := time.Now().UTC()
	id := e.Model.GetId()

	// Publish the stored row, not the Go model: custom models hide columns
	// (json:"-", e.g. stays.start_pending) and auth records hide system
	// fields, which would reach BigQuery as NULL. The stored row is also what
	// the reconciliation snapshot exports, so both sides compare equal.
	// After-hooks run once the transaction is committed, so the row is final.
	var data map[string]interface{}
	if operation != "delete" {
		row, err := storedRow(app.Dao().DB(), table, id)
		if err != nil {
			streamLog.Warn("failed to read stored row for streaming, using the model",
				"table", table, "id", id, "error", err)
		}
		data = row
	}
	if data == nil {
		var err error
		if data, err = modelData(e.Model); err != nil {
			streamLog.Warn("failed to serialize model for streaming",
				"table", table, "id", id, "error", err)
			return
		}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				streamLog.Warn("streaming goroutine panic recovered",
					"table", table, "id", id, "operation", operation, "panic", r)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := p.PublishModelChangeAt(ctx, table, operation, data, eventTime); err != nil {
			streamLog.Warn("failed to stream model change",
				"table", table, "id", id, "operation", operation, "error", err)
			return
		}

		streamLog.Debug("streamed model change", "table", table, "id", id, "operation", operation)
	}()
}

// storedRow reads a row exactly as stored in SQLite (every column, values as
// text, NULL as nil). Returns nil when the row does not exist.
func storedRow(db dbx.Builder, table, id string) (map[string]interface{}, error) {
	row := dbx.NullStringMap{}
	err := db.Select("*").From(table).Where(dbx.HashExp{"id": id}).Limit(1).One(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{}, len(row))
	for k, v := range row {
		if v.Valid {
			out[k] = v.String
		} else {
			out[k] = nil
		}
	}
	return out, nil
}

// modelData serializes a model as a generic map (fallback for deletes, when
// the row no longer exists).
func modelData(m interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// PublishStoredRows publishes an event with the stored row of every id, for
// changes made with raw SQL, which bypasses the model hooks. Call it once the
// change is committed: the rows are read right away (so the event carries
// that state) and published in the background. It never fails the caller:
// errors are only logged, and the nightly reconciliation fixes missed events.
func PublishStoredRows(db dbx.Builder, table, operation string, ids ...string) {
	defer func() {
		if r := recover(); r != nil {
			streamLog.Warn("PublishStoredRows panic recovered", "table", table, "panic", r)
		}
	}()

	p := GetPublisher()
	if p == nil || len(ids) == 0 || !p.ShouldStream(table) {
		return
	}
	eventTime := time.Now().UTC()

	rows := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		row, err := storedRow(db, table, id)
		if err != nil {
			streamLog.Warn("failed to read stored row for streaming", "table", table, "id", id, "error", err)
			continue
		}
		if row != nil {
			rows = append(rows, row)
		}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				streamLog.Warn("PublishStoredRows goroutine panic recovered", "table", table, "panic", r)
			}
		}()
		for _, row := range rows {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := p.PublishModelChangeAt(ctx, table, operation, row, eventTime)
			cancel()
			if err != nil {
				streamLog.Warn("failed to stream stored row", "table", table, "id", row["id"], "operation", operation, "error", err)
			}
		}
	}()
}
