# Plan: CH-13 — Shopping List UI (categories, checkboxes, manual remove)

## Summary

Build the shopping-list screen for the active weekly plan: a server-rendered page,
reachable from the home screen and the header nav, that groups the plan's
consolidated items under the 6 store categories (localized headings, fixed display
order). Each item carries an HTMX checkbox that persists its `checked` state to the
DB and a remove button that soft-removes the item (`manually_removed`) with an inline
undo. A client-side "show purchased" toggle hides checked items. All state lives on
the server; the checkbox/remove writes are **idempotent** (they set an absolute
target state, not a toggle) so a Service-Worker replay of an offline write is safe.

The backend builder (CH-12) and the `shopping_list_item` table already exist — this
story adds the read/write repository methods for item state, a new handler + routes,
templates + i18n, CSS, and the nav entry points. **No migration is needed**: the
`checked` and `manually_removed` columns are already in `000001_init.up.sql`.

## User Story

As a member of the household
I want to see the shopping list grouped by store section, tick off what I've bought, and remove items I don't need
So that I can move through the shop quickly without re-reading the whole list

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | MEDIUM |
| Systems Affected | handler, repository, templates, i18n, static (CSS) |
| GitHub Issue | #13 (CH-13) |
| Blocked by | CH-12 (shopping builder) — ✅ already merged |

---

## Design Decisions (read before implementing)

1. **No new service layer.** The read/write of item state is plain persistence with
   no business logic, so the handler depends on **narrow repository interfaces**
   directly — exactly as `recipeHandlers` depends on `recipeReader.GetRecipe`
   (`internal/handler/recipe.go:16-18`). `internal/service/shopping.go` is the
   generation-side *builder* and is unrelated to this screen.

2. **Read the active plan via the repository, not the generation service.** The
   shopping screen must work even when no LLM is wired (`canGenerate == false`).
   `service.GenerationService.CurrentPlan` is only constructed inside the
   `if canGenerate` block (`router.go:51-60`), so the handler instead calls
   `(*repository.Store).CurrentWeeklyPlan` (`weeklyplan.go:79-97`), which already
   returns the plan **with** its `ShoppingList`. The handler is always wired.

3. **Idempotent writes — absolute state, never a toggle.** Per the AC, a SW may
   replay an offline write. The check endpoint reads the desired state from the
   request (`r.FormValue("checked") == "true"`) and does `UPDATE ... SET checked = ?`.
   Applying it twice is a no-op. Remove (`SET manually_removed = 1`) and restore
   (`SET manually_removed = 0`) are likewise absolute. **Do not implement check/remove
   as a read-modify-write toggle** — that breaks under replay.

4. **HTMX swap model.** Each item is rendered by a reusable `shopping/item` partial.
   The checkbox and remove/undo actions target `closest .shopping-item` and swap
   `outerHTML` with the server-rendered partial, keeping the server the source of
   truth for the `--checked` class (which the "show purchased" toggle keys off).

5. **Removed items.** `manually_removed = 1` rows are filtered out of the grouped
   view on a fresh `GET /shopping`. Immediately after a remove, the item's slot is
   swapped for a small "removed — undo" stub so undo is available in-session
   (satisfies «с возможностью отменить»).

6. **"Show purchased" toggle is a view preference, not server state.** Implement it
   as a small progressive-enhancement JS toggle (mirroring the
   `static/js/cooking-steps.js` pattern) that toggles a class on the list container;
   CSS hides `.shopping-item--checked` when the list is in "hide purchased" mode.

7. **SW offline replay is out of scope to *build* here.** `static/sw.js:28-32` only
   intercepts GET and has no POST queue/Background-Sync. This story delivers the
   **idempotent server contract** that a future SW replay depends on; actual offline
   queueing is not implemented now. Recorded as a deferred item (below).

---

## Patterns to Follow

### Narrow repository interface on a handler (no service needed)
```go
// SOURCE: internal/handler/recipe.go:16-24
type recipeReader interface {
	GetRecipe(ctx context.Context, id string) (*domain.Recipe, error)
}
type recipeHandlers struct {
	rd      *renderer
	recipes recipeReader
}
```

### Repository UPDATE: timeout, parameterized query, requireOneRow
```go
// SOURCE: internal/repository/household.go:116-141 (+ requireOneRow 156-166)
func (s *Store) UpdateHousehold(ctx context.Context, h *domain.HouseholdProfile) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	const q = `UPDATE household_profile SET language = ?, ... WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, ...)
	if err != nil {
		return fmt.Errorf("update household: %w", err)
	}
	return requireOneRow(res, "update household") // 0 rows -> ErrNotFound
}
```

### Reading shopping items (scan + enum cast) — mirror for GetShoppingItem
```go
// SOURCE: internal/repository/weeklyplan.go:285-312
const q = `SELECT id, name, amount, unit, category, checked, manually_removed
	FROM shopping_list_item WHERE weekly_plan_id = ? ORDER BY rowid`
...
item.Category = domain.IngredientCategory(category)
```

### POST handler → render a fragment (HTTP 200, HTMX swaps it)
```go
// SOURCE: internal/handler/generate.go:78-101 ; render.go:72-91
gh.rd.renderFragment(w, r, http.StatusOK, "generate/cards", cardsData{Cards: cards})
```
- Form errors → 400 fragment; internal errors → `rd.fail(...)` (500, detail logged
  not leaked); see `render.go:101-104`.

### Page + content template pair, t() and HTMX attributes
```gohtml
{{/* SOURCE: templates/recipe.gohtml:1-63, home.gohtml:18-47, base.gohtml:11-16 */}}
{{- define "shopping/page" -}} ... {{ template "shopping/content" . }} ... {{- end -}}
{{- define "shopping/content" -}} ... {{- end -}}
{{/* i18n: {{ t "shopping.heading" }} ; with arg: {{ t "recipe.cook_time" .CookTime }} */}}
{{/* HTMX: hx-post="/shopping/item/{{.ID}}/check" hx-target="closest .shopping-item" hx-swap="outerHTML" */}}
```

### i18n keys are flat, dot-namespaced; %d/%s are Sprintf args
```json
// SOURCE: i18n/en.json (category.* already present)
"category.produce": "Produce",
"recipe.cook_time": "%d min"
```

### Handler test (stub) + repository test (real SQLite)
```go
// SOURCE: internal/handler/recipe_test.go:78-96 (HTMX fragment assertion),
//         internal/handler/generate_test.go:72-82 (stub wiring),
//         internal/repository/household_test.go:12-71 + db_test.go:10-25 (newTestStore)
req.Header.Set("HX-Request", "true") // fragment, assert no "<!doctype"
store := newTestStore(t)             // fresh SQLite + RunMigrations
```

### Progressive-enhancement JS toggle (for "show purchased")
```js
// SOURCE: pattern of static/js/cooking-steps.js (loaded with `defer`, no framework)
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/repository/shopping_item.go` | CREATE | `GetShoppingItem`, `SetShoppingItemChecked`, `SetShoppingItemRemoved` (idempotent UPDATEs) |
| `internal/repository/shopping_item_test.go` | CREATE | Real-SQLite tests incl. idempotent double-apply, ErrNotFound |
| `internal/handler/shopping.go` | CREATE | `shoppingHandlers`: GET list page + POST check/remove/restore; view models + grouping |
| `internal/handler/shopping_test.go` | CREATE | Stub-based tests: grouping, empty state, fragment vs page, idempotent check, undo, 3 languages, no leak |
| `templates/shopping.gohtml` | CREATE | `shopping/page`, `shopping/content`, `shopping/item`, `shopping/removed` partials |
| `static/js/shopping-filter.js` | CREATE | "Show purchased" client toggle (progressive enhancement) |
| `static/css/app.css` (or existing stylesheet) | UPDATE | `.shopping-*` styles, hide `.shopping-item--checked` in hide mode; Nordic Kitchen + ≥18pt/44px targets |
| `internal/handler/router.go` | UPDATE | Wire `shoppingHandlers`; register `GET /shopping`, `POST /shopping/item/{id}/check|remove|restore` |
| `templates/base.gohtml` | UPDATE | Add shopping-list link to header nav (accessible "from menu") |
| `templates/home.gohtml` | UPDATE | Add link/button to the shopping list (accessible "from home") |
| `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json` | UPDATE | `shopping.*` and `nav.shopping` keys (category.* already exist) |

> Confirm the stylesheet path during Task 6 (check `templates/base.gohtml` / `static/`
> for the existing CSS link; reuse it rather than introducing a new file if one exists).

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Repository — item read + idempotent state writes
- **File**: `internal/repository/shopping_item.go`
- **Action**: CREATE
- **Implement**:
  - `GetShoppingItem(ctx, id string) (*domain.ShoppingListItem, error)` — `SELECT id, name, amount, unit, category, checked, manually_removed FROM shopping_list_item WHERE id = ?`; scan, cast category, `ErrNotFound` on `sql.ErrNoRows`.
  - `SetShoppingItemChecked(ctx, id string, checked bool) error` — `UPDATE shopping_list_item SET checked = ? WHERE id = ?`; `requireOneRow`.
  - `SetShoppingItemRemoved(ctx, id string, removed bool) error` — `UPDATE shopping_list_item SET manually_removed = ? WHERE id = ?`; `requireOneRow`.
  - Each: `context.WithTimeout(ctx, queryTimeout)`, `fmt.Errorf("...: %w", err)` wrapping.
- **Mirror**: `internal/repository/household.go:116-141` (UPDATE + `requireOneRow`); `internal/repository/weeklyplan.go:285-312` (scan + enum cast).
- **Validate**: `gofmt -s -l . && go vet ./... && go build ./...`

### Task 2: Repository tests
- **File**: `internal/repository/shopping_item_test.go`
- **Action**: CREATE
- **Implement**: with `newTestStore(t)`, create a household + weekly plan with a couple of `ShoppingListItem`s (use `CreateWeeklyPlan`, `weeklyplan.go:17-24`). Then assert:
  - `SetShoppingItemChecked(id, true)` then `GetShoppingItem` → `Checked == true`; call it **twice** → still `true`, no error (idempotent).
  - `SetShoppingItemChecked(id, false)` → `false`.
  - `SetShoppingItemRemoved(id, true)` / `(id, false)` round-trips `ManuallyRemoved`.
  - missing id → `errors.Is(err, ErrNotFound)` for all three methods.
- **Mirror**: `internal/repository/household_test.go:12-71`, `db_test.go:10-25`.
- **Validate**: `go test ./internal/repository/...`

### Task 3: Handler — view models, grouping, list page
- **File**: `internal/handler/shopping.go`
- **Action**: CREATE
- **Implement**:
  - Narrow interfaces: `shoppingStore` (`CurrentWeeklyPlan`, `GetShoppingItem`, `SetShoppingItemChecked`, `SetShoppingItemRemoved`) and reuse a `householdProfiles`-style `Current(ctx, lang)` for the household ID (same interface the generate handler uses; confirm its name in `generate.go`).
  - `shoppingHandlers{ rd *renderer; store shoppingStore; households <currentHousehold> }`.
  - View models: `shoppingData{ Lang string; Empty bool; Groups []categoryGroup }`, `categoryGroup{ HeadingKey string; Items []shoppingItemView }`, `shoppingItemView{ ID, Name, Amount (formatted), Unit string; Checked bool }`. Reuse `formatAmount` (`recipe.go:103-108`).
  - `List(w, r)` (GET /shopping): resolve lang; `households.Current`; `store.CurrentWeeklyPlan(ctx, h.ID)`. On `ErrNotFound` → render with `Empty: true`. Build groups in **`categoryKeys` order** (`home.go:7-14`), skipping `ManuallyRemoved` items; map each item's `domain.IngredientCategory` to its `category.*` key. Render `"shopping"` via `rd.render`.
- **Mirror**: `internal/handler/recipe.go:57-98` (load → view projection → render; ErrNotFound branch); `internal/handler/home.go:32-39`.
- **Validate**: `go build ./... && go vet ./...`

### Task 4: Handler — check / remove / restore endpoints
- **File**: `internal/handler/shopping.go`
- **Action**: UPDATE (same file as Task 3)
- **Implement**:
  - `Check(w, r)` (POST /shopping/item/{id}/check): `id := r.PathValue("id")`; `checked := r.FormValue("checked") == "true"`; `store.SetShoppingItemChecked`. On `ErrNotFound` → `renderStatus(... 404 ...)` or a benign 200 (item gone is acceptable under replay — prefer 200 + empty to keep replay idempotent; pick 200 with `hx-swap` no-op). On success, `GetShoppingItem`, project to `shoppingItemView`, `renderFragment(w, r, 200, "shopping/item", ...)`.
  - `Remove(w, r)` (POST /shopping/item/{id}/remove): `SetShoppingItemRemoved(id, true)`; render `"shopping/removed"` stub fragment (carries `ID` so undo can post).
  - `Restore(w, r)` (POST /shopping/item/{id}/restore): `SetShoppingItemRemoved(id, false)`; `GetShoppingItem` → render `"shopping/item"` fragment.
  - Internal errors → `rd.fail(...)`; never leak `repository:`/`sql` detail.
- **Mirror**: `internal/handler/generate.go:78-101` (POST → `renderFragment`); `render.go:72-104`.
- **Validate**: `go build ./...`

### Task 5: Templates
- **File**: `templates/shopping.gohtml`
- **Action**: CREATE
- **Implement**:
  - `shopping/page` — full doc (mirror `recipe/page` head/header/scripts), `<main id="content">{{ template "shopping/content" . }}</main>`, load `htmx.min.js` + `<script src="/static/js/shopping-filter.js" defer></script>` + `sw-register`.
  - `shopping/content` — `<h1>{{ t "shopping.heading" }}</h1>`; if `.Empty` → friendly `{{ t "shopping.empty" }}` + link to home/generate; else a "show purchased" toggle control (checkbox/button wired to the JS) and, per group with items, `<h2>{{ t .HeadingKey }}</h2>` + `<ul>` of `{{ template "shopping/item" . }}`.
  - `shopping/item` — `<li class="shopping-item {{ if .Checked }}shopping-item--checked{{ end }}">` containing:
    - checkbox `<input type="checkbox" name="checked" value="true" {{ if .Checked }}checked{{ end }} hx-post="/shopping/item/{{ .ID }}/check" hx-trigger="change" hx-target="closest .shopping-item" hx-swap="outerHTML">` with a 44×44 label, amount/unit/name.
    - remove `<button hx-post="/shopping/item/{{ .ID }}/remove" hx-target="closest .shopping-item" hx-swap="outerHTML">{{ t "shopping.remove" }}</button>`.
  - `shopping/removed` — `<li class="shopping-item shopping-item--removed">{{ t "shopping.removed" }} <button hx-post="/shopping/item/{{ .ID }}/restore" hx-target="closest .shopping-item" hx-swap="outerHTML">{{ t "shopping.undo" }}</button></li>`.
- **Mirror**: `templates/recipe.gohtml:1-63`, `templates/home.gohtml:18-47`, `templates/base.gohtml:11-26`.
- **Validate**: `go test ./internal/handler/...` (template parse is covered by handler tests).

### Task 6: CSS + "show purchased" JS
- **File**: `static/js/shopping-filter.js` (CREATE) and the existing stylesheet (UPDATE)
- **Action**: CREATE + UPDATE
- **Implement**:
  - JS: on toggle change, add/remove a class (e.g. `shopping-list--hide-purchased`) on the list container. No framework; `defer`-loaded; guard for missing elements.
  - CSS (Nordic Kitchen): `.shopping-item` rows, ≥18pt body / ≥24pt headings, 44×44px touch targets, terracotta accent, cream bg, `prefers-color-scheme` already handled globally; `.shopping-list--hide-purchased .shopping-item--checked { display: none; }`; muted `.shopping-item--removed`.
- **Mirror**: `static/js/cooking-steps.js` (toggle pattern); existing category/`.recipe` styles in the current stylesheet.
- **Validate**: `gofmt -s -l . ` (static assets aren't compiled; verify visually deferred — see Verification table).

### Task 7: Wire routes + nav entry points
- **File**: `internal/handler/router.go`, `templates/base.gohtml`, `templates/home.gohtml`
- **Action**: UPDATE
- **Implement**:
  - In `NewRouter`: construct `sh := &shoppingHandlers{rd: rd, store: store, households: svc}` (unconditional — not inside the `canGenerate` block). Register:
    - `mux.HandleFunc("GET /shopping", sh.List)`
    - `mux.HandleFunc("POST /shopping/item/{id}/check", sh.Check)`
    - `mux.HandleFunc("POST /shopping/item/{id}/remove", sh.Remove)`
    - `mux.HandleFunc("POST /shopping/item/{id}/restore", sh.Restore)`
  - `base.gohtml` header: add `<a href="/shopping" hx-get="/shopping" hx-target="#content" hx-push-url="true">{{ t "nav.shopping" }}</a>` alongside the settings link.
  - `home.gohtml`: add a link/button to `/shopping` (HTMX-navigated, same pattern).
- **Mirror**: `internal/handler/router.go:39-63`; `templates/base.gohtml:11-16`.
- **Validate**: `go build ./... && go test ./internal/handler/...`

### Task 8: i18n keys (all three languages)
- **File**: `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json`
- **Action**: UPDATE
- **Implement**: add `nav.shopping`, `shopping.heading`, `shopping.empty`, `shopping.empty_hint`, `shopping.show_purchased`, `shopping.remove`, `shopping.undo`, `shopping.removed` to all three files (RU is the household's primary language — translate properly, don't leave English). `category.*` already exist — do not duplicate.
- **Mirror**: existing key blocks in `i18n/en.json`.
- **Validate**: `go test ./internal/i18n/... ./internal/handler/...`

### Task 9: Handler tests
- **File**: `internal/handler/shopping_test.go`
- **Action**: CREATE
- **Implement**: stub `shoppingStore` + stub household. Assert:
  - `GET /shopping` full page renders 6 localized category headings only for non-empty groups, checkboxes, and item names; HTMX (`HX-Request: true`) returns a fragment (no `<!doctype>`).
  - No active plan → empty-state message, status 200.
  - Manually-removed items are absent from the grouped list.
  - `POST /shopping/item/{id}/check` with `checked=true` → 200, fragment has `shopping-item--checked`; calling twice → still 200 (idempotent); `checked` absent/false → unchecked fragment.
  - `POST .../remove` → undo stub with restore button; `POST .../restore` → item fragment.
  - All three languages via `Accept-Language` (en/fi/ru) render localized headings.
  - Repository error path doesn't leak `repository:`/`sql` strings.
- **Mirror**: `internal/handler/recipe_test.go:28-114, 156-187`, `internal/handler/generate_test.go:72-123, 176-208`.
- **Validate**: `go test ./internal/handler/...`

### Task 10: Full validation pass
- **File**: —
- **Action**: run the full suite below; fix any `gofmt`/`vet`/`golangci-lint`/test findings.
- **Validate**: see Validation section.

---

## Risks

| Risk | Mitigation |
|------|------------|
| Implementing check/remove as a read-modify-write **toggle** would break SW replay idempotency | Endpoints set an **absolute** state from the request (`checked=true/false`; remove=1/restore=0). Tested with a double-apply in Task 2 & 9. |
| Coupling the screen to the generation service makes it unavailable when no LLM is wired | Handler reads `(*Store).CurrentWeeklyPlan` directly and is wired **unconditionally** in `NewRouter` (outside the `canGenerate` block). |
| SW offline POST queueing doesn't exist (`sw.js` GET-only) — AC implies replay | Out of scope to build now; deliver the idempotent server contract it depends on. Recorded as deferred (Verification table). |
| `ErrNotFound` on check after the row was removed could 500 a replayed write | Treat item-gone on check as a benign no-op (200), not a 500. |
| Stylesheet path / household-interface name assumed | Confirm both during Tasks 6/3 by reading `base.gohtml` and `generate.go` before editing. |
| `category.*` keys already exist | Reuse them; only add `shopping.*` + `nav.shopping`. Don't duplicate keys. |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .`, `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (incl. handler fragment + repo idempotency) | yes | — |
| Static `CGO_ENABLED=0` build | yes | — |
| `govulncheck ./...` | no (vuln.go.dev 403; no new deps anyway) | CH-21 deploy gate / networked host |
| HTMX checkbox/remove/undo over real browser | no (needs running server + Safari) | tailnet HTTPS / Mac mini, manual smoke |
| Service-Worker offline replay of checkbox writes | no (needs HTTPS + SW POST queue not built) | Deferred — future SW work; verified at CH-21 / tailnet HTTPS |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
# govulncheck ./...      # only if deps change (blocked in sandbox -> CH-21)
```

---

## Acceptance Criteria

- [ ] Shopping list screen reachable from home and from header nav
- [ ] Items grouped under the 6 categories with localized headings, fixed order
- [ ] Checkbox per item; tap persists `checked` to DB via HTMX; **idempotent**
- [ ] "Show purchased" toggle hides/shows checked items
- [ ] Remove button soft-removes (`manually_removed`) with inline undo
- [ ] State persists on the server; write endpoints are safe to replay
- [ ] No internal error/SQL detail leaked to the client
- [ ] All tasks completed; `gofmt`/`vet`/`golangci-lint`/`go test` pass
- [ ] Renders correctly in en/fi/ru
- [ ] Environment-blocked verifications (SW offline replay, browser smoke, govulncheck) recorded with their CH-21 / tailnet gate
```