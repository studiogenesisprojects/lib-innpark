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
func streamModelChange(app core.App, e *core.ModelEvent, operation string) {
	p := GetPublisher()
	if p == nil || e == nil || e.Model == nil {
		return
	}

	table := e.Model.TableName()
	if !p.ShouldStream(table) {
		return
	}

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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := p.PublishModelChange(ctx, table, operation, data); err != nil {
			app.Logger().Warn("failed to stream model change",
				"table", table, "id", id, "operation", operation, "error", err)
			return
		}

		app.Logger().Debug("streamed model change",
			"table", table, "id", id, "operation", operation)
	}()
}
