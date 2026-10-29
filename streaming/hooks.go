package streaming

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

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

	jsonData, err := json.Marshal(e.Model)
	if err != nil {
		streamLog.Warn("failed to serialize model for streaming",
			"table", table, "id", e.Model.GetId(), "error", err)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		streamLog.Warn("failed to unmarshal model data for streaming",
			"table", table, "id", e.Model.GetId(), "error", err)
		return
	}

	id := e.Model.GetId()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				streamLog.Warn("streaming goroutine panic recovered",
					"table", table, "id", id, "operation", operation, "panic", r)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := p.PublishModelChange(ctx, table, operation, data); err != nil {
			streamLog.Warn("failed to stream model change",
				"table", table, "id", id, "operation", operation, "error", err)
			return
		}

		streamLog.Debug("streamed model change", "table", table, "id", id, "operation", operation)
	}()
}
