package streaming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/pubsub"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	// publishAttempts is how many times a message publish is retried.
	publishAttempts = 3
	// topicRetention is how long undelivered messages stay in the topic.
	topicRetention = 7 * 24 * time.Hour
)

// Publisher publishes model changes to Pub/Sub, from where a native
// BigQuery subscription appends them to the events tables.
type Publisher struct {
	app       core.App
	cfg       *config
	client    *pubsub.Client
	bq        *bigquery.Client
	logger    *slog.Logger
	envPrefix string

	mu          sync.RWMutex
	topics      map[string]*pubsub.Topic
	schemas     map[string]bigquery.Schema
	provisioned map[string]bool
}

var (
	publisherInstance *Publisher
	publisherOnce     sync.Once
	publisherErr      error
)

// InitPublisher initializes the global publisher. Call it once at startup.
func InitPublisher(app core.App, opts Options) error {
	publisherOnce.Do(func() {
		cfg, err := loadConfig(opts)
		if err != nil {
			publisherErr = err
			return
		}

		ctx := context.Background()

		var client *pubsub.Client
		var bq *bigquery.Client
		if cfg.credsFile != "" {
			client, err = pubsub.NewClient(ctx, cfg.project, option.WithCredentialsFile(cfg.credsFile))
		} else {
			client, err = pubsub.NewClient(ctx, cfg.project)
		}
		if err != nil {
			publisherErr = fmt.Errorf("failed to create Pub/Sub client: %w", err)
			return
		}
		if cfg.credsFile != "" {
			bq, err = bigquery.NewClient(ctx, cfg.project, option.WithCredentialsFile(cfg.credsFile))
		} else {
			bq, err = bigquery.NewClient(ctx, cfg.project)
		}
		if err != nil {
			publisherErr = fmt.Errorf("failed to create BigQuery client: %w", err)
			return
		}

		publisherInstance = &Publisher{
			app:         app,
			cfg:         cfg,
			client:      client,
			bq:          bq,
			logger:      app.Logger(),
			envPrefix:   cfg.envPrefix,
			topics:      map[string]*pubsub.Topic{},
			schemas:     map[string]bigquery.Schema{},
			provisioned: map[string]bool{},
		}

		app.Logger().Info("streaming publisher initialized",
			"project", cfg.project, "dataset", cfg.dataset, "env", cfg.envPrefix)
	})

	return publisherErr
}

// GetPublisher returns the singleton publisher instance (nil if not initialized).
func GetPublisher() *Publisher {
	return publisherInstance
}

// ShouldStream reports whether the table must be streamed to BigQuery.
func (p *Publisher) ShouldStream(table string) bool {
	if isInternalTable(table) || p.cfg.opts.isExcludedTable(table) {
		return false
	}
	return true
}

// eventsTable returns the destination BigQuery table for a source table.
func (p *Publisher) eventsTable(table string) string {
	if tc := p.cfg.opts.tableConfig(table); tc.BQTable != "" {
		return tc.BQTable
	}
	return table + "_events"
}

// topicName returns the environment-specific topic name.
// Example: "stays" -> "dev-stays".
func (p *Publisher) topicName(table string) string {
	base := table
	if tc := p.cfg.opts.tableConfig(table); tc.TopicName != "" {
		base = tc.TopicName
	}
	return p.envPrefix + "-" + base
}

// PublishModelChange publishes one create/update/delete event for a model.
// The payload carries the same row content the old CSV exporter sent, plus
// the operation and the event timestamp.
func (p *Publisher) PublishModelChange(ctx context.Context, table, operation string, data map[string]interface{}) error {
	if !p.ShouldStream(table) {
		return nil
	}

	schema, err := p.eventsSchema(ctx, table)
	if err != nil {
		return fmt.Errorf("resolve schema for %s: %w", table, err)
	}

	filtered := filterSchemaFields(schema, data)
	coerceTypes(schema, filtered)
	convertJSONFields(schema, filtered)
	if schemaHas(schema, fieldOperation) {
		filtered[fieldOperation] = operation
		filtered[fieldEventTimestamp] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	payload, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	topic, err := p.ensureInfra(ctx, table)
	if err != nil {
		return fmt.Errorf("ensure infrastructure for %s: %w", table, err)
	}

	msg := &pubsub.Message{
		Data: payload,
		Attributes: map[string]string{
			"table":     table,
			"operation": operation,
		},
	}

	var lastErr error
	for attempt := 1; attempt <= publishAttempts; attempt++ {
		if _, err := topic.Publish(ctx, msg).Get(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		p.logger.Warn("publish attempt failed",
			"table", table, "operation", operation, "attempt", attempt, "error", err)
		time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
	}
	return fmt.Errorf("failed to publish %s event for %s: %w", operation, table, lastErr)
}

// EnsureEventsInfra creates the topic, events table and BigQuery subscription
// for a table if they do not exist yet. Exposed for the cutover scripts.
func (p *Publisher) EnsureEventsInfra(ctx context.Context, table string) error {
	_, err := p.ensureInfra(ctx, table)
	return err
}

// ensureInfra lazily provisions topic, events table and subscription and
// returns the cached topic handle.
func (p *Publisher) ensureInfra(ctx context.Context, table string) (*pubsub.Topic, error) {
	name := p.topicName(table)
	tc := p.cfg.opts.tableConfig(table)

	p.mu.RLock()
	topic, ok := p.topics[name]
	provisioned := p.provisioned[table]
	p.mu.RUnlock()

	if tc.SkipProvision {
		if !ok {
			topic = p.client.Topic(name)
			p.mu.Lock()
			p.topics[name] = topic
			p.mu.Unlock()
		}
		return topic, nil
	}

	if provisioned && ok {
		return topic, nil
	}

	if !ok {
		existing := p.client.Topic(name)
		exists, err := existing.Exists(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check topic %s: %w", name, err)
		}
		if exists {
			topic = existing
		} else {
			topic, err = p.client.CreateTopic(ctx, name)
			if err != nil {
				return nil, fmt.Errorf("failed to create topic %s: %w", name, err)
			}
			if _, err := topic.Update(ctx, pubsub.TopicConfigToUpdate{RetentionDuration: topicRetention}); err != nil {
				p.logger.Warn("failed to set topic retention", "topic", name, "error", err)
			}
			p.logger.Info("created Pub/Sub topic", "topic", name)
		}
		p.mu.Lock()
		p.topics[name] = topic
		p.mu.Unlock()
	}

	if err := p.ensureEventsTable(ctx, table); err != nil {
		return nil, err
	}
	if err := p.ensureSubscription(ctx, table, name); err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.provisioned[table] = true
	p.mu.Unlock()

	return topic, nil
}

// ensureEventsTable creates the events table (partitioned by day of created,
// clustered by id) if it does not exist.
func (p *Publisher) ensureEventsTable(ctx context.Context, table string) error {
	schema, err := p.eventsSchema(ctx, table)
	if err != nil {
		return err
	}

	t := p.bq.Dataset(p.cfg.dataset).Table(p.eventsTable(table))
	if _, err := t.Metadata(ctx); err == nil {
		return nil
	} else if !isNotFound(err) {
		return fmt.Errorf("failed to check events table %s: %w", p.eventsTable(table), err)
	}

	meta := &bigquery.TableMetadata{Schema: schema}
	if schemaHas(schema, "created") {
		meta.TimePartitioning = &bigquery.TimePartitioning{
			Field: "created",
			Type:  bigquery.DayPartitioningType,
		}
	}
	if schemaHas(schema, "id") {
		meta.Clustering = &bigquery.Clustering{Fields: []string{"id"}}
	}

	if err := t.Create(ctx, meta); err != nil {
		return fmt.Errorf("failed to create events table %s: %w", p.eventsTable(table), err)
	}
	p.logger.Info("created BigQuery events table",
		"dataset", p.cfg.dataset, "table", p.eventsTable(table))
	return nil
}

// ensureSubscription creates the native Pub/Sub -> BigQuery subscription.
// Unlike the legacy subscription, it never expires.
func (p *Publisher) ensureSubscription(ctx context.Context, table, topicName string) error {
	id := topicName + "-bq"
	sub := p.client.Subscription(id)
	if exists, err := sub.Exists(ctx); err != nil {
		return fmt.Errorf("failed to check subscription %s: %w", id, err)
	} else if exists {
		return nil
	}

	_, err := p.client.CreateSubscription(ctx, id, pubsub.SubscriptionConfig{
		Topic: p.client.Topic(topicName),
		BigQueryConfig: pubsub.BigQueryConfig{
			Table:          fmt.Sprintf("%s.%s.%s", p.cfg.project, p.cfg.dataset, p.eventsTable(table)),
			UseTableSchema: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create BigQuery subscription %s: %w", id, err)
	}
	p.logger.Info("created BigQuery subscription", "subscription", id)
	return nil
}

// eventsSchema resolves the schema of the events table for a table:
//  1. the events table itself, if it already exists;
//  2. the legacy BigQuery table with the same name (pre-cutover), plus the
//     event envelope;
//  3. the SQLite source table (PRAGMA table_info), plus the event envelope.
//
// Using the existing BigQuery schema keeps the events tables byte-compatible
// with what the consumers already query.
func (p *Publisher) eventsSchema(ctx context.Context, table string) (bigquery.Schema, error) {
	p.mu.RLock()
	s, ok := p.schemas[table]
	p.mu.RUnlock()
	if ok {
		return s, nil
	}

	ds := p.bq.Dataset(p.cfg.dataset)

	if md, err := ds.Table(p.eventsTable(table)).Metadata(ctx); err == nil && md.Type == bigquery.RegularTable {
		s := pruneExcludedColumns(table, md.Schema, p.cfg.opts.ExcludeColumns)
		p.cacheSchema(table, s)
		return s, nil
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}

	if p.eventsTable(table) != table {
		if md, err := ds.Table(table).Metadata(ctx); err == nil && md.Type == bigquery.RegularTable {
			s := withEventFields(pruneExcludedColumns(table, md.Schema, p.cfg.opts.ExcludeColumns))
			p.cacheSchema(table, s)
			return s, nil
		} else if err != nil && !isNotFound(err) {
			return nil, err
		}
	}

	var cols []sqliteColumn
	q := p.app.Dao().DB().NewQuery(`SELECT name, type FROM pragma_table_info({:t})`).
		Bind(dbx.Params{"t": table})
	if err := q.All(&cols); err != nil {
		return nil, fmt.Errorf("failed to read SQLite columns of %s: %w", table, err)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %s not found in BigQuery nor in SQLite", table)
	}

	s = schemaFromSQLiteColumns(table, cols, p.cfg.opts.ExcludeColumns)
	p.cacheSchema(table, s)
	return s, nil
}

func (p *Publisher) cacheSchema(table string, s bigquery.Schema) {
	p.mu.Lock()
	p.schemas[table] = s
	p.mu.Unlock()
}

// withEventFields appends the event envelope fields if missing.
func withEventFields(s bigquery.Schema) bigquery.Schema {
	out := make(bigquery.Schema, 0, len(s)+2)
	out = append(out, s...)
	for _, f := range eventFields() {
		if !schemaHas(out, f.Name) {
			out = append(out, f)
		}
	}
	return out
}

// filterSchemaFields keeps only the fields present in the BigQuery schema.
func filterSchemaFields(schema bigquery.Schema, data map[string]interface{}) map[string]interface{} {
	filtered := make(map[string]interface{}, len(data))
	for _, f := range schema {
		if v, ok := data[f.Name]; ok {
			filtered[f.Name] = v
		}
	}
	return filtered
}

// coerceTypes normalizes JSON values to the BigQuery column types
// (booleans and whole floats into INTEGER columns).
func coerceTypes(schema bigquery.Schema, data map[string]interface{}) {
	for _, f := range schema {
		if f.Type != bigquery.IntegerFieldType {
			continue
		}
		switch v := data[f.Name].(type) {
		case bool:
			if v {
				data[f.Name] = 1
			} else {
				data[f.Name] = 0
			}
		case float64:
			data[f.Name] = int64(v)
		}
	}
}

// convertJSONFields stringifies values of JSON-type columns so BigQuery
// accepts them.
func convertJSONFields(schema bigquery.Schema, data map[string]interface{}) {
	for _, f := range schema {
		if f.Type != bigquery.JSONFieldType {
			continue
		}
		v, ok := data[f.Name]
		if !ok || v == nil {
			continue
		}
		if _, isString := v.(string); isString {
			continue
		}
		if b, err := json.Marshal(v); err == nil {
			data[f.Name] = string(b)
		}
	}
}

func schemaHas(schema bigquery.Schema, name string) bool {
	for _, f := range schema {
		if f.Name == name {
			return true
		}
	}
	return false
}

// pruneExcludedColumns removes sensitive columns from a schema, so they are
// neither stored in the events tables nor published in the payloads.
func pruneExcludedColumns(table string, s bigquery.Schema, exclusions []string) bigquery.Schema {
	out := make(bigquery.Schema, 0, len(s))
	for _, f := range s {
		if isExcludedColumn(table, f.Name, exclusions) {
			continue
		}
		out = append(out, f)
	}
	return out
}

func isNotFound(err error) bool {
	var gerr *googleapi.Error
	return errors.As(err, &gerr) && gerr.Code == 404
}

// Close stops the publisher and releases resources.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.topics {
		t.Stop()
	}
	if err := p.client.Close(); err != nil {
		return err
	}
	return p.bq.Close()
}
