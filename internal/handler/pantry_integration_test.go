package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/static"
)

// TestPantryEndToEnd drives the pantry-basics flow through the real router and a
// real (in-memory, migrated) SQLite store: show seeds localized defaults → add →
// confirm persisted → add duplicate (deduped) → remove → confirm persisted gone.
// This exercises route wiring, the real household service, the repository's JSON
// (de)serialization of pantry_basics, templates, and i18n together (HTTP-level
// E2E per CLAUDE.md).
func TestPantryEndToEnd(t *testing.T) {
	db, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repository.RunMigrations(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := repository.New(db)
	ctx := context.Background()
	srv := NewRouter(slogDiscard(), db, testBundle(t), testTemplates(t), static.FS, nil)

	// 1. Show creates the household with the localized default list (EN default).
	if body := doGet(t, srv, "/settings/pantry"); !strings.Contains(body, "salt") || !strings.Contains(body, "sugar") {
		t.Fatalf("default pantry basics not rendered:\n%s", body)
	}
	h, err := store.FirstHousehold(ctx)
	if err != nil {
		t.Fatalf("first household: %v", err)
	}
	base := len(h.PantryBasics)
	if base == 0 {
		t.Fatal("defaults not seeded on creation")
	}

	// 2. Add a new staple; it appears in the fragment and is persisted.
	if code := doPostForm(t, srv, "/settings/pantry/add", "item=olive+oil"); code != 200 {
		t.Fatalf("add status = %d, want 200", code)
	}
	h, _ = store.FirstHousehold(ctx)
	if len(h.PantryBasics) != base+1 || !containsStr(h.PantryBasics, "olive oil") {
		t.Fatalf("add not persisted: %v", h.PantryBasics)
	}

	// 3. Adding a case-insensitive duplicate leaves the list unchanged.
	if code := doPostForm(t, srv, "/settings/pantry/add", "item=Olive+Oil"); code != 200 {
		t.Fatalf("dup add status = %d, want 200", code)
	}
	h, _ = store.FirstHousehold(ctx)
	if len(h.PantryBasics) != base+1 {
		t.Fatalf("duplicate persisted: %v", h.PantryBasics)
	}

	// 4. Remove a seeded staple; it is dropped and persisted.
	if code := doPostForm(t, srv, "/settings/pantry/remove", "item=salt"); code != 200 {
		t.Fatalf("remove status = %d, want 200", code)
	}
	h, _ = store.FirstHousehold(ctx)
	if containsStr(h.PantryBasics, "salt") {
		t.Fatalf("removed item still persisted: %v", h.PantryBasics)
	}
}

func containsStr(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
