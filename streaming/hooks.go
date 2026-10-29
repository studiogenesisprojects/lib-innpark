package streaming

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// RegisterHooks streams every model change (create, update and delete) of
// the tables enabled for streaming. Call it once at startup, after
// InitPublisher.
func RegisterHooks(app core.App) {
	app.Logger().Info("etl-debug: registering streaming hooks")
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
	app.Logger().Info("etl-debug: hook invoked", "operation", operation)
	defer func() {
		if r := recover(); r != nil {
			app.Logger().Warn("streaming hook panic recovered",
				"operation", operation, "panic", r)
		}
	}()

	p := GetPublisher()
	if p == nil || e == nil || e.Model == nil {
		app.Logger().Warn("etl-debug: skipped (nil)",
			"operation", operation, "publisher", p != nil, "event", e != nil)
		return
	}

	table := e.Model.TableName()
	if !p.ShouldStream(table) {
		if !isInternalTable(table) {
			app.Logger().Info("etl-debug: table not streamed", "table", table, "operation", operation)
		}
		return
	}

	app.Logger().Info("etl-debug: publishing", "table", table, "id", e.Model.GetId(), "operation", operation)

	jsonData, err := json.Marshal(e.Model)
	if err != nil {
		app.Logger().Warn("failed to serialize model for streaming",
			"table", table, "id", e.Model.GetId(), "error", err)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		app.Logger().Warn("failed to unmarshal model data for streaming",
			"table", table, "id", e.Model.GetId(), "error", err)
		return
	}

	id := e.Model.GetId()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				app.Logger().Warn("streaming goroutine panic recovered",
					"table", table, "id", id, "operation", operation, "panic", r)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := p.PublishModelChange(ctx, table, operation, data); err != nil {
			app.Logger().Warn("failed to stream model change",
				"table", table, "id", id, "operation", operation, "error", err)
			return
		}

		app.Logger().Info("etl-debug: published", "table", table, "id", id, "operation", operation)
	}()
}
