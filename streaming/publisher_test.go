package streaming

import (
	"errors"
	"testing"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsAlreadyExists(t *testing.T) {
	if !isAlreadyExists(status.Error(codes.AlreadyExists, "resource already exists")) {
		t.Error("expected AlreadyExists to be detected")
	}
	if isAlreadyExists(status.Error(codes.Internal, "boom")) {
		t.Error("expected Internal not to be treated as AlreadyExists")
	}
	if isAlreadyExists(errors.New("plain error")) {
		t.Error("expected plain errors not to be treated as AlreadyExists")
	}
	if isAlreadyExists(nil) {
		t.Error("expected nil not to be treated as AlreadyExists")
	}
}

func TestNormalizeTimestamps(t *testing.T) {
	schema := bigquery.Schema{
		{Name: "created", Type: bigquery.TimestampFieldType},
		{Name: "updated", Type: bigquery.TimestampFieldType},
		{Name: "due_at", Type: bigquery.TimestampFieldType},
		{Name: "status", Type: bigquery.StringFieldType},
	}
	data := map[string]interface{}{
		"created": "2026-10-29 10:43:24.355Z", // PocketBase format: keep
		"updated": "",                         // zero types.DateTime: drop
		"due_at":  "not-a-date",               // unparseable: drop
		"status":  "ACTIVE",
	}

	normalizeTimestamps(schema, data)

	if _, ok := data["created"]; !ok {
		t.Error("expected PocketBase-formatted timestamp to be kept")
	}
	if _, ok := data["updated"]; ok {
		t.Error("expected empty timestamp to be dropped")
	}
	if _, ok := data["due_at"]; ok {
		t.Error("expected unparseable timestamp to be dropped")
	}
	if data["status"] != "ACTIVE" {
		t.Error("expected non-timestamp fields to be untouched")
	}
}

func TestStringifyComplexFields(t *testing.T) {
	schema := bigquery.Schema{
		{Name: "settings", Type: bigquery.StringFieldType}, // Prod BQ: STRING
		{Name: "logos", Type: bigquery.StringFieldType},    // multi-file: STRING
		{Name: "payload", Type: bigquery.JSONFieldType},    // JSON column: leave to convertJSONFields
		{Name: "name", Type: bigquery.StringFieldType},
	}
	data := map[string]interface{}{
		"settings": map[string]interface{}{"lang": "es"},
		"logos":    []interface{}{"logo1.png", "logo2.png"},
		"payload":  map[string]interface{}{"keep": "object"},
		"name":     "plain",
	}

	stringifyComplexFields(schema, data)

	if got, ok := data["settings"].(string); !ok || got != `{"lang":"es"}` {
		t.Errorf("settings = %#v, want JSON string", data["settings"])
	}
	if got, ok := data["logos"].(string); !ok || got != `["logo1.png","logo2.png"]` {
		t.Errorf("logos = %#v, want JSON string", data["logos"])
	}
	if _, ok := data["payload"].(map[string]interface{}); !ok {
		t.Error("JSON-typed columns must be left untouched")
	}
	if data["name"] != "plain" {
		t.Error("plain strings must be untouched")
	}
}

func TestTopicNameIsServiceScoped(t *testing.T) {
	onstreet := &Publisher{envPrefix: "dev", cfg: &config{service: "onstreet", opts: Options{}}}
	payment := &Publisher{envPrefix: "dev", cfg: &config{service: "payment", opts: Options{}}}

	if a, b := onstreet.topicName("users"), payment.topicName("users"); a == b {
		t.Fatalf("different services must not share a topic, both got %q", a)
	} else if a != "dev-onstreet-users" || b != "dev-payment-users" {
		t.Fatalf("unexpected topic names: %q / %q", a, b)
	}

	legacy := &Publisher{envPrefix: "prod", cfg: &config{service: "offstreet", opts: Options{
		Tables: map[string]TableConfig{"integration_requests": {TopicName: "integration-requests"}},
	}}}
	if got := legacy.topicName("integration_requests"); got != "prod-integration-requests" {
		t.Fatalf("legacy override broken: %q", got)
	}
}

func TestIsAlreadyExistsBigQuery409(t *testing.T) {
	err := &googleapi.Error{Code: 409, Message: "Already Exists: Table x"}
	if !isAlreadyExists(err) {
		t.Error("expected googleapi 409 (BigQuery duplicate table) to be detected")
	}
	if isAlreadyExists(&googleapi.Error{Code: 403, Message: "forbidden"}) {
		t.Error("expected googleapi 403 not to be treated as AlreadyExists")
	}
}

func TestSanitizeByType(t *testing.T) {
	schema := bigquery.Schema{
		{Name: "created", Type: bigquery.TimestampFieldType},
		{Name: "mobile_phone", Type: bigquery.IntegerFieldType}, // BQ INTEGER, PB sends ""
		{Name: "counter", Type: bigquery.IntegerFieldType},
		{Name: "amount", Type: bigquery.FloatFieldType},
		{Name: "flag", Type: bigquery.BooleanFieldType},
		{Name: "name", Type: bigquery.StringFieldType},
	}
	data := map[string]interface{}{
		"created":      "",
		"mobile_phone": "",
		"counter":      "42",
		"amount":       "12.5",
		"flag":         "true",
		"name":         "",
	}

	sanitizeByType(schema, data)

	if _, ok := data["created"]; ok {
		t.Error("empty timestamp must be dropped")
	}
	if _, ok := data["mobile_phone"]; ok {
		t.Error("empty string for INTEGER must be dropped")
	}
	if data["counter"] != int64(42) {
		t.Errorf("numeric string for INTEGER must be coerced, got %#v", data["counter"])
	}
	if data["amount"] != 12.5 {
		t.Errorf("numeric string for FLOAT must be coerced, got %#v", data["amount"])
	}
	if data["flag"] != true {
		t.Errorf("boolean string must be coerced, got %#v", data["flag"])
	}
	if data["name"] != "" {
		t.Error("empty string for STRING must be kept")
	}
}
