package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
)

// shoppingStore is the slice of repository.Store the shopping handler needs, kept
// narrow so the handler is testable with a stub. *repository.Store satisfies it.
type shoppingStore interface {
	CurrentWeeklyPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)
	GetShoppingItem(ctx context.Context, id string) (*domain.ShoppingListItem, error)
	SetShoppingItemChecked(ctx context.Context, id string, checked bool) error
	SetShoppingItemRemoved(ctx context.Context, id string, removed bool) error
}

// shoppingHandlers serve the shopping-list screen (F-3 / CH-13): the category-
// grouped list plus the per-item check / remove / restore actions. Reads come
// straight from the repository so the screen works even when no LLM is wired.
type shoppingHandlers struct {
	rd         *renderer
	store      shoppingStore
	households householdProfiles
}

// categoryKeyByID maps a store category to its i18n heading key. Unknown
// categories fall back to "category.other".
var categoryKeyByID = map[domain.IngredientCategory]string{
	domain.CategoryProduce:  "category.produce",
	domain.CategoryMeatFish: "category.meat_fish",
	domain.CategoryDairy:    "category.dairy",
	domain.CategoryPantry:   "category.pantry",
	domain.CategoryFrozen:   "category.frozen",
	domain.CategoryOther:    "category.other",
}

// shoppingData is the view model for the shopping-list screen.
type shoppingData struct {
	Lang   string
	Empty  bool
	Groups []categoryGroup
}

// categoryGroup is one store-section block: a localized heading key and its items.
type categoryGroup struct {
	HeadingKey string
	Items      []shoppingItemView
}

// shoppingItemView is the template-facing projection of a ShoppingListItem.
// Amount is pre-formatted so the template never formats numbers.
type shoppingItemView struct {
	ID      string
	Name    string
	Amount  string
	Unit    string
	Checked bool
}

// List renders the shopping list for the household's active plan, grouped by
// store category in the canonical category order (PRD §15 Appendix). When the
// household has no active plan, a friendly empty state is rendered. Manually
// removed items are filtered out of the grouped view.
func (sh *shoppingHandlers) List(w http.ResponseWriter, r *http.Request) {
	lang := string(LanguageFromContext(r.Context()))

	h, err := sh.households.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		sh.rd.fail(w, r, "load household for shopping list", err)
		return
	}

	plan, err := sh.store.CurrentWeeklyPlan(r.Context(), h.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			sh.rd.render(w, r, "shopping", shoppingData{Lang: lang, Empty: true})
			return
		}
		sh.rd.fail(w, r, "load current plan for shopping list", err)
		return
	}

	groups := groupShoppingItems(plan.ShoppingList)
	sh.rd.render(w, r, "shopping", shoppingData{
		Lang:   lang,
		Empty:  len(groups) == 0,
		Groups: groups,
	})
}

// groupShoppingItems buckets items by store category in categoryKeys order,
// skipping manually-removed lines and dropping empty groups. Item order within a
// group preserves the plan's order.
func groupShoppingItems(items []domain.ShoppingListItem) []categoryGroup {
	byCategory := make(map[string][]shoppingItemView)
	for _, item := range items {
		if item.ManuallyRemoved {
			continue
		}
		key := categoryKeyFor(item.Category)
		byCategory[key] = append(byCategory[key], toShoppingItemView(item))
	}

	groups := make([]categoryGroup, 0, len(categoryKeys))
	for _, key := range categoryKeys {
		if views := byCategory[key]; len(views) > 0 {
			groups = append(groups, categoryGroup{HeadingKey: key, Items: views})
		}
	}
	return groups
}

// categoryKeyFor returns the i18n heading key for a category, defaulting to
// "category.other" for an empty or unrecognized value.
func categoryKeyFor(c domain.IngredientCategory) string {
	if key, ok := categoryKeyByID[c]; ok {
		return key
	}
	return "category.other"
}

// toShoppingItemView projects a domain item into its template shape.
func toShoppingItemView(item domain.ShoppingListItem) shoppingItemView {
	return shoppingItemView{
		ID:      item.ID,
		Name:    item.Name,
		Amount:  formatAmount(item.Amount),
		Unit:    item.Unit,
		Checked: item.Checked,
	}
}

// Check handles POST /shopping/item/{id}/check: it persists the absolute checked
// state carried in the request (checkbox present => true) and re-renders the item
// row. The write is idempotent, so a replayed offline write is safe. An item that
// no longer exists is treated as a benign no-op (the row may have been removed
// since the offline write was queued) rather than an error.
func (sh *shoppingHandlers) Check(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	checked := r.FormValue("checked") == "true"

	if err := sh.store.SetShoppingItemChecked(r.Context(), id, checked); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			w.WriteHeader(http.StatusOK)
			return
		}
		sh.rd.fail(w, r, "set shopping item checked", err)
		return
	}
	sh.renderItem(w, r, id)
}

// Remove handles POST /shopping/item/{id}/remove: it soft-removes the item
// (manually_removed = true) and renders an inline "removed — undo" stub so the
// action can be reverted in-session.
func (sh *shoppingHandlers) Remove(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := sh.store.SetShoppingItemRemoved(r.Context(), id, true); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			w.WriteHeader(http.StatusOK)
			return
		}
		sh.rd.fail(w, r, "remove shopping item", err)
		return
	}
	sh.rd.renderFragment(w, r, http.StatusOK, "shopping/removed", shoppingItemView{ID: id})
}

// Restore handles POST /shopping/item/{id}/restore: it clears the manually-removed
// flag and re-renders the item row.
func (sh *shoppingHandlers) Restore(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := sh.store.SetShoppingItemRemoved(r.Context(), id, false); err != nil {
		sh.rd.fail(w, r, "restore shopping item", err)
		return
	}
	sh.renderItem(w, r, id)
}

// renderItem reloads an item by ID and renders its row partial. A vanished item
// degrades to an empty 200 (HTMX swaps in nothing) rather than a 500.
func (sh *shoppingHandlers) renderItem(w http.ResponseWriter, r *http.Request, id string) {
	item, err := sh.store.GetShoppingItem(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			w.WriteHeader(http.StatusOK)
			return
		}
		sh.rd.fail(w, r, "reload shopping item", err)
		return
	}
	sh.rd.renderFragment(w, r, http.StatusOK, "shopping/item", toShoppingItemView(*item))
}
