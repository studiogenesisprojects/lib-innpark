package innpark

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNovuAPIBaseURL(t *testing.T) {
	t.Setenv("NOVU_API_URL", "http://10.204.0.37:3000/v1/")

	baseURL, err := novuAPIBaseURL()
	if err != nil {
		t.Fatalf("novuAPIBaseURL returned an error: %v", err)
	}
	if got, want := baseURL.String(), "http://10.204.0.37:3000"; got != want {
		t.Fatalf("novuAPIBaseURL = %q, want %q", got, want)
	}
}

func TestNovuAPIBaseURLDefaultsToCloud(t *testing.T) {
	t.Setenv("NOVU_API_URL", "")

	baseURL, err := novuAPIBaseURL()
	if err != nil {
		t.Fatalf("novuAPIBaseURL returned an error: %v", err)
	}
	if got, want := baseURL.String(), "https://api.novu.co"; got != want {
		t.Fatalf("novuAPIBaseURL = %q, want %q", got, want)
	}
}

func TestNovuAPIBaseURLRejectsInvalidValues(t *testing.T) {
	t.Setenv("NOVU_API_URL", "://invalid")

	if _, err := novuAPIBaseURL(); err == nil {
		t.Fatal("novuAPIBaseURL accepted an invalid value")
	}
}

func TestCreateSubscriberDefaultsLocaleToSpanish(t *testing.T) {
	var requestBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/v1/subscribers"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()

	t.Setenv("NOVU_API_URL", server.URL)
	t.Setenv("NOVU_TOKEN", "test-token")

	if err := CreateSubscriber("user-123", "user@example.com"); err != nil {
		t.Fatalf("CreateSubscriber returned an error: %v", err)
	}
	if got, want := requestBody["subscriberId"], "user-123"; got != want {
		t.Errorf("subscriberId = %q, want %q", got, want)
	}
	if got, want := requestBody["email"], "user@example.com"; got != want {
		t.Errorf("email = %q, want %q", got, want)
	}
	if got, want := requestBody["locale"], DEFAULT_SUBSCRIBER_LOCALE; got != want {
		t.Errorf("locale = %q, want %q", got, want)
	}
}
