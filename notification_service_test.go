package innpark

import "testing"

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
