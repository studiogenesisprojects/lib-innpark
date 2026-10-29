package streaming

import (
	"fmt"
	"os"
	"strings"
)

// TableConfig holds per-table streaming overrides.
type TableConfig struct {
	// BQTable overrides the destination BigQuery table (default "<table>_events").
	BQTable string
	// TopicName overrides the base topic name (default the table name).
	// Needed for legacy topics like "integration-requests".
	TopicName string
	// SkipProvision disables automatic creation of topic, table and
	// subscription. Use it when the infrastructure already exists and is
	// managed elsewhere (e.g. the legacy integration_requests flow).
	SkipProvision bool
}

// Options configures the streaming package for one API.
type Options struct {
	// ExcludeTables must not be streamed (in addition to internal tables).
	ExcludeTables []string
	// ExcludeColumns are columns that must never leave the database.
	// Each entry is "column" (for any table) or "table.column".
	ExcludeColumns []string
	// KeepFileFields streams PocketBase file fields. They are excluded by
	// default: they only reference binaries and are of no analytical value.
	KeepFileFields bool
	// DeadLetterTopic is the topic used for messages that cannot be written
	// after MaxDeliveryAttempts (default "<env>-deadletter"). A message that
	// fails forever blocks the subscription, so this is not optional in
	// production. The topic must exist and grant publish to the Pub/Sub
	// service agent; otherwise subscriptions are created without it.
	DeadLetterTopic string
	// MaxDeliveryAttempts is the delivery cap before a message is sent to the
	// dead-letter topic. Defaults to 5.
	MaxDeliveryAttempts int
	// Tables holds per-table overrides.
	Tables map[string]TableConfig
}

type config struct {
	project   string
	dataset   string
	envPrefix string
	service   string
	credsFile string
	opts      Options
}

// loadConfig reads the environment configuration shared by all APIs.
//
//	BQ_DATASET_NAME    (required) e.g. "dev_offstreet" or "prod_offstreet"
//	GOOGLE_CLOUD_PROJECT (optional) defaults to "innpark"
func loadConfig(opts Options) (*config, error) {
	dataset := os.Getenv("BQ_DATASET_NAME")
	if dataset == "" {
		return nil, fmt.Errorf("BQ_DATASET_NAME environment variable is required")
	}

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		project = "innpark"
	}

	credsFile := ""
	if _, err := os.Stat("serviceAccountKey.json"); err == nil {
		credsFile = "serviceAccountKey.json"
	}

	return &config{
		project:   project,
		dataset:   dataset,
		envPrefix: extractEnvPrefix(dataset),
		service:   extractService(dataset),
		credsFile: credsFile,
		opts:      opts,
	}, nil
}

// extractEnvPrefix returns the environment prefix from the dataset name.
// Examples: "dev_offstreet" -> "dev", "prod_payment" -> "prod".
func extractEnvPrefix(dataset string) string {
	if i := strings.Index(dataset, "_"); i > 0 {
		return dataset[:i]
	}
	return "default"
}

// extractService returns the service part of the dataset name.
// Examples: "dev_offstreet" -> "offstreet", "prod_payment" -> "payment".
// It keeps topics of different APIs apart: onstreet and payment both have a
// "users" table and must not share a topic.
func extractService(dataset string) string {
	if i := strings.Index(dataset, "_"); i > 0 && i+1 < len(dataset) {
		return dataset[i+1:]
	}
	return dataset
}

// maxDeliveryAttempts returns the configured cap (default 5).
func (o Options) maxDeliveryAttempts() int {
	if o.MaxDeliveryAttempts > 0 {
		return o.MaxDeliveryAttempts
	}
	return 5
}

// isExcludedTable reports whether the table is excluded from streaming.
func (o Options) isExcludedTable(table string) bool {
	for _, t := range o.ExcludeTables {
		if t == table {
			return true
		}
	}
	return false
}

// tableConfig returns the overrides for a table (zero value if none).
func (o Options) tableConfig(table string) TableConfig {
	return o.Tables[table]
}
