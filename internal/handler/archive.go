package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// archiveLister supplies the household's recipes for the archive list and the
// search-as-you-type fragment. *repository.Store satisfies it.
type archiveLister interface {
	SearchRecipes(ctx context.Context, householdID, query string, limit int) ([]domain.Recipe, error)
}

// cookAgainService replays an archived recipe into the household's current plan.
// *service.GenerationService satisfies it. It is nil when no LLM is wired, in
// which case the "cook again" action is disabled (the shopping-list rebuild may
// fall through to an LLM categorize call).
type cookAgainService interface {
	CurrentPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)
	CookAgain(ctx context.Context, h *domain.HouseholdProfile, plan *domain.WeeklyPlan, oldRecipeID, sourceRecipeID string) (*service.SwappedRecipe, error)
}

// archiveLimit bounds how many recipes the archive screen loads at once. A single
// household's history stays comfortably under this for the foreseeable future.
const archiveLimit = 200

// archiveHandlers serve the recipe archive (F-8 / CH-18): a newest-first list of
// every recipe the household has generated, substring title search, and a
// "cook again" action that replays a recipe into the current week. Listing and
// search are pure reads and always available; cook again is wired only when the
// LLM is configured (canCook), mirroring the per-card swap.
type archiveHandlers struct {
	rd         *renderer
	recipes    archiveLister
	households householdProfiles
	plans      recipeLoader
	cook       cookAgainService // nil when the LLM is not wired
	canCook    bool
}

// archiveData is the view model for the archive screen and its list fragment.
type archiveData struct {
	Lang      string
	CanCook   bool
	Query     string
	LoadError bool // a read failed; render a friendly banner instead of 500-ing
	Recipes   []archiveRow
}

// archiveRow is the template-facing projection of a recipe in the archive list.
// The three feedback flags drive the read-only icons; no interactive control is
// rendered here.
type archiveRow struct {
	ID          string
	Title       string
	Description string
	CookTime    int
	Liked       bool
	Disliked    bool
	CookAgain   bool
}

// cookAgainDialogData is the view model for the "replace which recipe?" dialog.
// SourceID is the archived recipe to replay; Current lists the active week's
// recipes the user can replace. NoPlan signals there is no active week.
type cookAgainDialogData struct {
	Lang     string
	SourceID string
	NoPlan   bool
	Current  []archiveRow
}

// cookAgainDoneData is the view model for the post-replay confirmation.
type cookAgainDoneData struct {
	Lang       string
	AddedTitle string
}

// toArchiveRow projects a domain recipe into the archive row shape.
func toArchiveRow(r domain.Recipe) archiveRow {
	row := archiveRow{
		ID:          r.ID,
		Title:       r.Title,
		Description: r.Description,
		CookTime:    r.CookTimeMinutes,
	}
	if r.Feedback != nil {
		row.Liked = r.Feedback.Liked
		row.Disliked = r.Feedback.Disliked
		row.CookAgain = r.Feedback.CookAgain
	}
	return row
}

// toArchiveRows projects a slice of recipes into archive rows.
func toArchiveRows(recipes []domain.Recipe) []archiveRow {
	rows := make([]archiveRow, len(recipes))
	for i, r := range recipes {
		rows[i] = toArchiveRow(r)
	}
	return rows
}

// Show renders the archive screen with every recipe (newest first). A failure to
// load the list degrades to a friendly in-page banner at HTTP 200 rather than a
// whole-page 500 (the detail is logged, never echoed to the client).
func (ah *archiveHandlers) Show(w http.ResponseWriter, r *http.Request) {
	lang := string(LanguageFromContext(r.Context()))

	h, err := ah.households.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ah.rd.fail(w, r, "load household for archive", err)
		return
	}

	data := archiveData{Lang: lang, CanCook: ah.canCook}
	recipes, err := ah.recipes.SearchRecipes(r.Context(), h.ID, "", archiveLimit)
	if err != nil {
		slog.Warn("archive list load failed", "err", err, "request_id", RequestIDFromContext(r.Context()))
		data.LoadError = true
		ah.rd.render(w, r, "archive", data)
		return
	}

	data.Recipes = toArchiveRows(recipes)
	ah.rd.render(w, r, "archive", data)
}

// Search handles GET /archive/search?q=…: it renders the list fragment filtered
// by a substring title match. A read failure degrades to the same in-page banner.
func (ah *archiveHandlers) Search(w http.ResponseWriter, r *http.Request) {
	lang := string(LanguageFromContext(r.Context()))

	h, err := ah.households.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ah.rd.fail(w, r, "load household for archive search", err)
		return
	}

	query := r.FormValue("q")
	data := archiveData{Lang: lang, CanCook: ah.canCook, Query: query}
	recipes, err := ah.recipes.SearchRecipes(r.Context(), h.ID, query, archiveLimit)
	if err != nil {
		slog.Warn("archive search failed", "err", err, "request_id", RequestIDFromContext(r.Context()))
		data.LoadError = true
		ah.rd.renderFragment(w, r, http.StatusOK, "archive/list", data)
		return
	}

	data.Recipes = toArchiveRows(recipes)
	ah.rd.renderFragment(w, r, http.StatusOK, "archive/list", data)
}

// CookAgainDialog handles GET /archive/cook-again/{id}: it renders the dialog
// asking which of the current week's recipes to replace with the archived recipe
// {id}. When there is no active week it renders a "no active week" state.
func (ah *archiveHandlers) CookAgainDialog(w http.ResponseWriter, r *http.Request) {
	lang := string(LanguageFromContext(r.Context()))

	sourceID := strings.TrimSpace(r.PathValue("id"))
	if sourceID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	h, err := ah.households.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ah.rd.fail(w, r, "load household for cook-again dialog", err)
		return
	}

	data := cookAgainDialogData{Lang: lang, SourceID: sourceID}
	plan, err := ah.cook.CurrentPlan(r.Context(), h.ID)
	if err != nil {
		ah.rd.fail(w, r, "load current plan for cook-again dialog", err)
		return
	}
	if plan == nil {
		data.NoPlan = true
		ah.rd.renderFragment(w, r, http.StatusOK, "archive/dialog", data)
		return
	}

	current, err := ah.plans.RecipesByIDs(r.Context(), plan.RecipeIDs)
	if err != nil {
		ah.rd.fail(w, r, "load current recipes for cook-again dialog", err)
		return
	}
	data.Current = toArchiveRows(current)
	ah.rd.renderFragment(w, r, http.StatusOK, "archive/dialog", data)
}

// CookAgain handles POST /archive/cook-again/{id}: it replaces the chosen current
// recipe (form field "old") with a fresh copy of the archived recipe {id} and
// rebuilds the shopping list, then renders a confirmation fragment. A vanished
// active week renders the dialog's "no active week" state; a service failure
// renders a localized error fragment (HTTP 200 so HTMX swaps it). Internal detail
// is logged, never sent to the client.
func (ah *archiveHandlers) CookAgain(w http.ResponseWriter, r *http.Request) {
	lang := string(LanguageFromContext(r.Context()))

	sourceID := strings.TrimSpace(r.PathValue("id"))
	oldID := strings.TrimSpace(r.FormValue("old"))
	if sourceID == "" || oldID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	h, err := ah.households.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ah.rd.fail(w, r, "load household for cook-again", err)
		return
	}

	plan, err := ah.cook.CurrentPlan(r.Context(), h.ID)
	if err != nil {
		ah.rd.fail(w, r, "load current plan for cook-again", err)
		return
	}
	if plan == nil {
		ah.rd.renderFragment(w, r, http.StatusOK, "archive/dialog",
			cookAgainDialogData{Lang: lang, SourceID: sourceID, NoPlan: true})
		return
	}

	swapped, err := ah.cook.CookAgain(r.Context(), h, plan, oldID, sourceID)
	if err != nil {
		slog.Warn("cook again failed", "err", err, "request_id", RequestIDFromContext(r.Context()))
		ah.rd.renderFragment(w, r, http.StatusOK, "archive/error", archiveData{Lang: lang})
		return
	}

	ah.rd.renderFragment(w, r, http.StatusOK, "archive/done",
		cookAgainDoneData{Lang: lang, AddedTitle: swapped.Recipe.Title})
}
