package streaming

import "testing"

func TestExtractEnvPrefix(t *testing.T) {
	cases := map[string]string{
		"dev_offstreet": "dev",
		"prod_onstreet": "prod",
		"prod_payment":  "prod",
		"staging_x_y":   "staging",
		"nodash":        "default",
	}
	for in, want := range cases {
		if got := extractEnvPrefix(in); got != want {
			t.Errorf("extractEnvPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractService(t *testing.T) {
	cases := map[string]string{
		"dev_offstreet": "offstreet",
		"prod_payment":  "payment",
		"prod_onstreet": "onstreet",
		"nodash":        "nodash",
	}
	for in, want := range cases {
		if got := extractService(in); got != want {
			t.Errorf("extractService(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShouldStream(t *testing.T) {
	p := &Publisher{cfg: &config{opts: Options{
		ExcludeTables: []string{"integration_requests"},
	}}}

	cases := map[string]bool{
		"stays":                true,
		"integration_requests": false, // excluded
		"admins":               false, // internal
		"_collections":         false, // internal
		"payments":             true,
	}
	for table, want := range cases {
		if got := p.ShouldStream(table); got != want {
			t.Errorf("ShouldStream(%q) = %v, want %v", table, got, want)
		}
	}
}

func TestEventsTableNaming(t *testing.T) {
	p := &Publisher{envPrefix: "default", cfg: &config{service: "onstreet", opts: Options{
		Tables: map[string]TableConfig{
			"integration_requests": {BQTable: "integration_requests", TopicName: "integration-requests", SkipProvision: true},
		},
	}}}

	if got := p.eventsTable("stays"); got != "stays_events" {
		t.Errorf("eventsTable(stays) = %q, want stays_events", got)
	}
	if got := p.eventsTable("integration_requests"); got != "integration_requests" {
		t.Errorf("eventsTable(integration_requests) = %q, want integration_requests", got)
	}
	if got := p.topicName("stays"); got != "default-onstreet-stays" {
		t.Errorf("topicName(stays) = %q, want default-onstreet-stays", got)
	}
	if got := p.topicName("integration_requests"); got != "default-integration-requests" {
		t.Errorf("topicName(integration_requests) = %q, want default-integration-requests", got)
	}
}
