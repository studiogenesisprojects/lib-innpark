package streaming

import (
	"errors"
	"testing"

	"cloud.google.com/go/bigquery"
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
