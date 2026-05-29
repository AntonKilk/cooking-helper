package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/static"
)

// TestShoppingEndToEnd drives the shopping flow through the real router and a real
// (in-memory, migrated) SQLite store: list → check → confirm persisted → remove →
// confirm gone → restore → confirm back. This exercises route wiring, the real
// repository, templates, and i18n together (HTTP-level E2E per CLAUDE.md).
func TestShoppingEndToEnd(t *testing.T) {
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

	h := &domain.HouseholdProfile{Language: domain.LanguageEN, FamilySize: domain.FamilySize{Adults: 2}}
	if err := store.CreateHousehold(ctx, h); err != nil {
		t.Fatalf("create household: %v", err)
	}
	plan := &domain.WeeklyPlan{
		HouseholdID: h.ID,
		WeekStart:   time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		RecipeIDs:   []string{"r1", "r2", "r3"},
		ShoppingList: []domain.ShoppingListItem{
			{Name: "Carrot", Amount: 3, Unit: "pcs", Category: domain.CategoryProduce},
			{Name: "Milk", Amount: 1, Unit: "l", Category: domain.CategoryDairy},
		},
	}
	if err := store.CreateWeeklyPlan(ctx, plan); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	carrotID := plan.ShoppingList[0].ID

	srv := NewRouter(slogDiscard(), db, testBundle(t), testTemplates(t), static.FS, nil)

	// 1. List shows the grouped items.
	if body := doGet(t, srv, "/shopping"); !strings.Contains(body, "Carrot") || !strings.Contains(body, "Produce") {
		t.Fatalf("list missing items/headings:\n%s", body)
	}

	// 2. Check the carrot.
	if code := doPostForm(t, srv, "/shopping/item/"+carrotID+"/check", "checked=true"); code != http.StatusOK {
		t.Fatalf("check status = %d, want 200", code)
	}
	got, err := store.GetShoppingItem(ctx, carrotID)
	if err != nil || !got.Checked {
		t.Fatalf("carrot not persisted checked: item=%+v err=%v", got, err)
	}

	// 3. Remove the carrot; it disappears from a fresh list (default hides nothing
	//    removed remains out of the grouped view).
	if code := doPostForm(t, srv, "/shopping/item/"+carrotID+"/remove", ""); code != http.StatusOK {
		t.Fatalf("remove status = %d, want 200", code)
	}
	if body := doGet(t, srv, "/shopping"); strings.Contains(body, "Carrot") {
		t.Fatalf("removed item still listed:\n%s", body)
	}

	// 4. Restore the carrot; it returns to the list.
	if code := doPostForm(t, srv, "/shopping/item/"+carrotID+"/restore", ""); code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200", code)
	}
	if body := doGet(t, srv, "/shopping"); !strings.Contains(body, "Carrot") {
		t.Fatalf("restored item not listed:\n%s", body)
	}
}

func doGet(t *testing.T, srv http.Handler, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", path, rec.Code)
	}
	return rec.Body.String()
}

func doPostForm(t *testing.T, srv http.Handler, path, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec.Code
}
