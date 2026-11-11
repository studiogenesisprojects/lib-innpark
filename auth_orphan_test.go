package innpark

import (
	"errors"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func newTestApp(t *testing.T, withDeletedAt bool) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	table := "CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, updated TEXT NOT NULL DEFAULT '')"
	if withDeletedAt {
		table = "CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, updated TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '')"
	}
	if _, err := app.DB().NewQuery(table).Execute(); err != nil {
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

func columnOf(t *testing.T, app core.App, column, id string) string {
	t.Helper()
	var value string
	if err := app.DB().Select(column).From("users").Where(dbx.HashExp{"id": id}).Row(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func releaseFor(t *testing.T, app core.App, email, newID string, exists func(string) (bool, error)) error {
	t.Helper()
	orphans, err := orphanEmailHolders(app.Dao(), "users", email, newID, exists)
	if err != nil {
		return err
	}
	return releaseOrphanEmails(app.Dao(), "users", orphans)
}

func TestReleaseOrphanEmailFreesAndSoftDeletesDeletedAccount(t *testing.T) {
	app := newTestApp(t, true)
	addUser(t, app, "orphanuid000001", "Ana@Example.com")
	addUser(t, app, "otheruser000001", "other@example.com")
	gone := func(string) (bool, error) { return false, nil }
	if err := releaseFor(t, app, "ana@example.com", "newuid000000001", gone); err != nil {
		t.Fatal(err)
	}
	if got := columnOf(t, app, "email", "orphanuid000001"); got != "deleted+orphanuid000001@invalid" {
		t.Fatalf("orphan email = %q", got)
	}
	if columnOf(t, app, "deleted_at", "orphanuid000001") == "" {
		t.Fatal("orphan must be soft-deleted")
	}
	if got := columnOf(t, app, "email", "otheruser000001"); got != "other@example.com" {
		t.Fatalf("unrelated user touched: %q", got)
	}
	if got := columnOf(t, app, "deleted_at", "otheruser000001"); got != "" {
		t.Fatalf("unrelated user soft-deleted: %q", got)
	}
	addUser(t, app, "newuid000000001", "ana@example.com") // the new account can now be created
}

func TestReleaseOrphanEmailKeepsExistingDeletedAt(t *testing.T) {
	app := newTestApp(t, true)
	addUser(t, app, "orphanuid000001", "ana@example.com")
	if _, err := app.DB().Update("users", dbx.Params{"deleted_at": "2026-01-01 00:00:00.000Z"}, dbx.HashExp{"id": "orphanuid000001"}).Execute(); err != nil {
		t.Fatal(err)
	}
	if err := releaseFor(t, app, "ana@example.com", "newuid000000001", func(string) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if got := columnOf(t, app, "deleted_at", "orphanuid000001"); got != "2026-01-01 00:00:00.000Z" {
		t.Fatalf("original deletion date overwritten: %q", got)
	}
}

func TestReleaseOrphanEmailWithoutDeletedAtColumn(t *testing.T) {
	app := newTestApp(t, false)
	addUser(t, app, "orphanuid000001", "ana@example.com")
	if err := releaseFor(t, app, "ana@example.com", "newuid000000001", func(string) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if got := columnOf(t, app, "email", "orphanuid000001"); got != "deleted+orphanuid000001@invalid" {
		t.Fatalf("orphan email = %q", got)
	}
}

func TestReleaseOrphanEmailRefusesLiveAccount(t *testing.T) {
	app := newTestApp(t, true)
	addUser(t, app, "liveuid00000001", "ana@example.com")
	live := func(string) (bool, error) { return true, nil }
	if err := releaseFor(t, app, "ana@example.com", "newuid000000001", live); err == nil {
		t.Fatal("expected an error for a live account holding the email")
	}
	if got := columnOf(t, app, "email", "liveuid00000001"); got != "ana@example.com" {
		t.Fatalf("live account email changed: %q", got)
	}
}

func TestReleaseOrphanEmailStopsOnLookupError(t *testing.T) {
	app := newTestApp(t, true)
	addUser(t, app, "orphanuid000001", "ana@example.com")
	broken := func(string) (bool, error) { return true, errors.New("firebase unavailable") }
	if err := releaseFor(t, app, "ana@example.com", "newuid000000001", broken); err == nil {
		t.Fatal("expected the lookup error")
	}
	if got := columnOf(t, app, "email", "orphanuid000001"); got != "ana@example.com" {
		t.Fatalf("email changed despite lookup error: %q", got)
	}
}

func TestReleaseOrphanEmailIgnoresSameIdAndEmpty(t *testing.T) {
	app := newTestApp(t, true)
	addUser(t, app, "sameuid00000001", "ana@example.com")
	calls := 0
	gone := func(string) (bool, error) { calls++; return false, nil }
	if err := releaseFor(t, app, "ana@example.com", "sameuid00000001", gone); err != nil {
		t.Fatal(err)
	}
	if err := releaseFor(t, app, "  ", "x", gone); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || columnOf(t, app, "email", "sameuid00000001") != "ana@example.com" {
		t.Fatalf("own record or empty email must be left alone (calls=%d)", calls)
	}
}
