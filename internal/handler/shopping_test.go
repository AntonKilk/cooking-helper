package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
)

// stubShoppingStore is an in-memory shoppingStore for the handler tests. It keeps
// items by ID so check/remove writes are observable through GetShoppingItem
// (exercising the re-render path). plan/planErr drive CurrentWeeklyPlan.
type stubShoppingStore struct {
	plan    *domain.WeeklyPlan
	planErr error
	items   map[string]*domain.ShoppingListItem
	getErr  error
	setErr  error
}

func (s *stubShoppingStore) CurrentWeeklyPlan(_ context.Context, _ string) (*domain.WeeklyPlan, error) {
	if s.planErr != nil {
		return nil, s.planErr
	}
	return s.plan, nil
}

func (s *stubShoppingStore) GetShoppingItem(_ context.Context, id string) (*domain.ShoppingListItem, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	item, ok := s.items[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return item, nil
}

func (s *stubShoppingStore) SetShoppingItemChecked(_ context.Context, id string, checked bool) error {
	if s.setErr != nil {
		return s.setErr
	}
	item, ok := s.items[id]
	if !ok {
		return repository.ErrNotFound
	}
	item.Checked = checked
	return nil
}

func (s *stubShoppingStore) SetShoppingItemRemoved(_ context.Context, id string, removed bool) error {
	if s.setErr != nil {
		return s.setErr
	}
	item, ok := s.items[id]
	if !ok {
		return repository.ErrNotFound
	}
	item.ManuallyRemoved = removed
	return nil
}

// Compile-time guards: the real store and the stub satisfy the narrow interface.
var (
	_ shoppingStore = (*repository.Store)(nil)
	_ shoppingStore = (*stubShoppingStore)(nil)
)

func newShoppingHandler(t *testing.T, store shoppingStore) *shoppingHandlers {
	t.Helper()
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	return &shoppingHandlers{rd: rd, store: store, households: stubHouseholds{}}
}

func testPlanWithItems() (*domain.WeeklyPlan, map[string]*domain.ShoppingListItem) {
	items := []domain.ShoppingListItem{
		{ID: "i1", Name: "Carrot", Amount: 3, Unit: "pcs", Category: domain.CategoryProduce},
		{ID: "i2", Name: "Milk", Amount: 1, Unit: "l", Category: domain.CategoryDairy, Checked: true},
		{ID: "i3", Name: "Pasta", Amount: 500, Unit: "g", Category: domain.CategoryPantry},
		{ID: "i4", Name: "Ghost", Amount: 1, Unit: "pcs", Category: domain.CategoryOther, ManuallyRemoved: true},
	}
	byID := make(map[string]*domain.ShoppingListItem, len(items))
	for i := range items {
		byID[items[i].ID] = &items[i]
	}
	return &domain.WeeklyPlan{ID: "p1", HouseholdID: "hh-1", ShoppingList: items}, byID
}

func getShopping(htmx bool) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodGet, "/shopping", nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	return httptest.NewRecorder(), req
}

func postItem(action, id, body string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/shopping/item/"+id+"/"+action, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	return httptest.NewRecorder(), req
}

func TestShoppingListGroupsByCategory(t *testing.T) {
	plan, byID := testPlanWithItems()
	h := newShoppingHandler(t, &stubShoppingStore{plan: plan, items: byID})
	rec, req := getShopping(false)

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Produce", "Dairy", "Pantry", // localized category headings
		"Carrot", "Milk", "Pasta", // item names
		`hx-post="/shopping/item/i1/check"`, // checkbox wiring
		"Show purchased",                    // filter toggle
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	// Manually-removed items must not appear in the grouped list.
	if strings.Contains(body, "Ghost") {
		t.Errorf("manually-removed item leaked into the list:\n%s", body)
	}
	// Category order: produce before pantry.
	if strings.Index(body, "Carrot") > strings.Index(body, "Pasta") {
		t.Error("expected produce (Carrot) to render before pantry (Pasta)")
	}
}

func TestShoppingListHTMXReturnsFragment(t *testing.T) {
	plan, byID := testPlanWithItems()
	h := newShoppingHandler(t, &stubShoppingStore{plan: plan, items: byID})
	rec, req := getShopping(true)

	h.List(rec, req)

	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Errorf("HTMX request returned a full page, want fragment:\n%s", body)
	}
	if !strings.Contains(body, "Carrot") {
		t.Errorf("fragment missing items:\n%s", body)
	}
}

func TestShoppingListEmpty(t *testing.T) {
	h := newShoppingHandler(t, &stubShoppingStore{planErr: repository.ErrNotFound})
	rec, req := getShopping(false)

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No shopping list yet.") {
		t.Errorf("body missing empty-state message:\n%s", body)
	}
}

func TestShoppingListRepositoryErrorIs500(t *testing.T) {
	h := newShoppingHandler(t, &stubShoppingStore{planErr: errors.New("db down")})
	rec, req := getShopping(false)

	h.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "db down") {
		t.Errorf("internal error leaked to body:\n%s", rec.Body.String())
	}
}

func TestShoppingCheckPersistsAndIsIdempotent(t *testing.T) {
	_, byID := testPlanWithItems()
	store := &stubShoppingStore{items: byID}
	h := newShoppingHandler(t, store)

	// Apply checked=true twice; both must succeed and leave the item checked.
	for i := 0; i < 2; i++ {
		rec, req := postItem("check", "i1", "checked=true")
		h.Check(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("check call %d status = %d, want 200", i+1, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "shopping-item--checked") {
			t.Errorf("call %d: rendered item missing checked class:\n%s", i+1, rec.Body.String())
		}
	}
	if !byID["i1"].Checked {
		t.Error("item should be checked after double-apply")
	}

	// Unchecking: no "checked" field in the body.
	rec, req := postItem("check", "i1", "")
	h.Check(rec, req)
	if byID["i1"].Checked {
		t.Error("item should be unchecked when checked field absent")
	}
	if strings.Contains(rec.Body.String(), "shopping-item--checked") {
		t.Errorf("rendered item should not be checked:\n%s", rec.Body.String())
	}
}

func TestShoppingCheckMissingItemIsBenign(t *testing.T) {
	h := newShoppingHandler(t, &stubShoppingStore{items: map[string]*domain.ShoppingListItem{}})
	rec, req := postItem("check", "gone", "checked=true")

	h.Check(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (replay of a write for a removed item is benign)", rec.Code)
	}
}

func TestShoppingRemoveThenRestore(t *testing.T) {
	_, byID := testPlanWithItems()
	store := &stubShoppingStore{items: byID}
	h := newShoppingHandler(t, store)

	rec, req := postItem("remove", "i3", "")
	h.Remove(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove status = %d, want 200", rec.Code)
	}
	if !byID["i3"].ManuallyRemoved {
		t.Error("item should be marked manually removed")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Removed") || !strings.Contains(body, "Undo") {
		t.Errorf("remove fragment missing removed/undo affordance:\n%s", body)
	}
	if !strings.Contains(body, `hx-post="/shopping/item/i3/restore"`) {
		t.Errorf("undo button missing restore wiring:\n%s", body)
	}

	rec, req = postItem("restore", "i3", "")
	h.Restore(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200", rec.Code)
	}
	if byID["i3"].ManuallyRemoved {
		t.Error("item should be restored")
	}
	if !strings.Contains(rec.Body.String(), "Pasta") {
		t.Errorf("restore fragment missing item name:\n%s", rec.Body.String())
	}
}

func TestShoppingListRendersInAllThreeLanguages(t *testing.T) {
	cases := []struct {
		lang    string
		heading string
		produce string
	}{
		{"en-US", "Shopping list", "Produce"},
		{"fi-FI", "Ostoslista", "Vihannekset ja hedelmät"},
		{"ru-RU", "Список покупок", "Овощи и фрукты"},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			plan, byID := testPlanWithItems()
			srv := newShoppingRouter(t, &stubShoppingStore{plan: plan, items: byID})
			req := httptest.NewRequest(http.MethodGet, "/shopping", nil)
			req.Header.Set("Accept-Language", c.lang)
			rec := httptest.NewRecorder()

			srv.ServeHTTP(rec, req)

			body := rec.Body.String()
			if !strings.Contains(body, c.heading) {
				t.Errorf("[%s] body missing heading %q", c.lang, c.heading)
			}
			if !strings.Contains(body, c.produce) {
				t.Errorf("[%s] body missing localized produce heading %q", c.lang, c.produce)
			}
		})
	}
}

// newShoppingRouter wires the shopping handler behind the language middleware so
// Accept-Language resolution can be exercised end-to-end.
func newShoppingRouter(t *testing.T, store shoppingStore) http.Handler {
	t.Helper()
	h := newShoppingHandler(t, store)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /shopping", h.List)
	return languageMiddleware(h.rd.bundle, mux)
}
