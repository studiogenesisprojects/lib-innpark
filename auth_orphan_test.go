package innpark

import (
	"errors"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func newTestApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if _, err := app.DB().NewQuery("CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, updated TEXT NOT NULL DEFAULT '')").Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DB().NewQuery("CREATE UNIQUE INDEX idx_users_email ON users (email COLLATE NOCASE)").Execute(); err != nil {
		t.Fatal(err)
	}
	return app
}

func addUser(t *testing.T, app core.App, id, email string) {
	t.Helper()
	if _, err := app.DB().Insert("users", dbx.Params{"id": id, "email": email}).Execute(); err != nil {
		t.Fatal(err)
	}
}

func emailOf(t *testing.T, app core.App, id string) string {
	t.Helper()
	var email string
	if err := app.DB().Select("email").From("users").Where(dbx.HashExp{"id": id}).Row(&email); err != nil {
		t.Fatal(err)
	}
	return email
}

func TestReleaseOrphanEmailFreesDeletedAccount(t *testing.T) {
	app := newTestApp(t)
	addUser(t, app, "orphanuid000001", "Ana@Example.com")
	addUser(t, app, "otheruser000001", "other@example.com")
	gone := func(string) (bool, error) { return false, nil }
	if err := releaseOrphanEmail(app.Dao(), "users", "ana@example.com", "newuid000000001", gone); err != nil {
		t.Fatal(err)
	}
	if got := emailOf(t, app, "orphanuid000001"); got != "deleted+orphanuid000001@invalid" {
		t.Fatalf("orphan email = %q", got)
	}
	if got := emailOf(t, app, "otheruser000001"); got != "other@example.com" {
		t.Fatalf("unrelated user touched: %q", got)
	}
	addUser(t, app, "newuid000000001", "ana@example.com") // the new account can now be created
}

func TestReleaseOrphanEmailRefusesLiveAccount(t *testing.T) {
	app := newTestApp(t)
	addUser(t, app, "liveuid00000001", "ana@example.com")
	live := func(string) (bool, error) { return true, nil }
	if err := releaseOrphanEmail(app.Dao(), "users", "ana@example.com", "newuid000000001", live); err == nil {
		t.Fatal("expected an error for a live account holding the email")
	}
	if got := emailOf(t, app, "liveuid00000001"); got != "ana@example.com" {
		t.Fatalf("live account email changed: %q", got)
	}
}

func TestReleaseOrphanEmailStopsOnLookupError(t *testing.T) {
	app := newTestApp(t)
	addUser(t, app, "orphanuid000001", "ana@example.com")
	broken := func(string) (bool, error) { return true, errors.New("firebase unavailable") }
	if err := releaseOrphanEmail(app.Dao(), "users", "ana@example.com", "newuid000000001", broken); err == nil {
		t.Fatal("expected the lookup error")
	}
	if got := emailOf(t, app, "orphanuid000001"); got != "ana@example.com" {
		t.Fatalf("email changed despite lookup error: %q", got)
	}
}

func TestReleaseOrphanEmailIgnoresSameIdAndEmpty(t *testing.T) {
	app := newTestApp(t)
	addUser(t, app, "sameuid00000001", "ana@example.com")
	calls := 0
	gone := func(string) (bool, error) { calls++; return false, nil }
	if err := releaseOrphanEmail(app.Dao(), "users", "ana@example.com", "sameuid00000001", gone); err != nil {
		t.Fatal(err)
	}
	if err := releaseOrphanEmail(app.Dao(), "users", "  ", "x", gone); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || emailOf(t, app, "sameuid00000001") != "ana@example.com" {
		t.Fatalf("own record or empty email must be left alone (calls=%d)", calls)
	}
}
