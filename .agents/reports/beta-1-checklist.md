# Cooking Helper — Beta Testing Checklist (Beta-1)

**Issue:** [#21](https://github.com/AntonKilk/cooking-helper/issues/21) (CH-21) · Phase 4 (Polish & Beta)
**Target:** MVP success metrics from PRD §11 · **Device:** iPad Safari on the kitchen counter, over tailnet HTTPS

This is the script the author's family follows during the 2-week trial. Each scenario
traces to a user story (CH-*) and/or a PRD §11 criterion. Record **PASS / FAIL / N/A**
and notes. Anything that fails becomes a bug in the log at the bottom (severity P0/P1/P2).

> **How to run:** Open the app fresh on the iPad over the tailnet HTTPS URL. Go top to
> bottom. For scenario 2, **use a stopwatch** — that number is the headline metric.

---

## A. Functional scenarios (≥10, traced to stories)

| # | Scenario | Story / PRD | Steps | Expected | Result | Notes |
|---|----------|-------------|-------|----------|--------|-------|
| 1 | **First-run onboarding** | CH-19 | Launch on a clean profile | 3–4 screens: (1) family size + language, (2) pantry basics (default acceptable), (3) generate→cook→feedback explainer. "Skip" works on every step. Does **not** reappear on next launch. | ☐ | |
| 2 | **One-tap week generation + time-to-list** | CH-8 / F-1 · **PRD §11** | From app open, generate a week and reach the shopping list. **Start stopwatch at open.** | Reachable in **≤ 2 taps**; generation completes **≤ 30 s**; **open → shopping list < 2 min**. Record the time. | ☐ | time: ____ |
| 3 | **Recipe swap & full regenerate** | CH-9 / F-2 | Swap one recipe; then full-regenerate the week | Single recipe swaps without touching the others; full regen replaces all 3. | ☐ | |
| 4 | **Disliked ingredient — 100% exclusion** | CH-10 / CH-15 / F-7 · **PRD §11** | Add a disliked ingredient (e.g. cilantro), regenerate | Ingredient appears in **none** of the 3 recipes **and not** in the shopping list. | ☐ | |
| 5 | **Pantry basics never in shopping list** | CH-14 / F-6 · **PRD §11** | Ensure salt/oil/etc. are in pantry basics, generate | Pantry-basic items **never** appear in the shopping list. | ☐ | |
| 6 | **Shopping list auto-categorization + check-off** | CH-12 / CH-13 / F-3 · **PRD §11** | Open shopping list; check several items; reload the page | Items grouped by store category **without manual tagging**; checked state **survives reload** (HTMX absolute-state). | ☐ | |
| 7 | **Fullscreen recipe during cooking** | CH-11 / F-4 · **PRD §11/UX** | Open a recipe, read at ~50 cm, step through cooking steps | Readable **without zoom**; step navigation works; no scroll-jacking. | ☐ | |
| 8 | **Feedback collection** | CH-16 / F-5 | Mark like / dislike / cook-again on recipes | Each feedback type is recorded and reflected in the UI. | ☐ | |
| 9 | **Feedback influences next generation** | CH-17 | Dislike a recipe, generate the next week | The disliked recipe (or its defining trait) is **not** repeated. | ☐ | |
| 10 | **Recipe archive — search & "cook again"** | CH-18 / F-8 | Open archive, search a past recipe, tap "cook again" | Search returns matches; "cook again" re-adds the recipe to a plan. | ☐ | |
| 11 | **i18n switch RU ↔ FI ↔ EN** | CH-4 / F-9 · **PRD §11** | Switch UI language across all three | **All** UI strings translate; **existing recipes keep their original language** (no re-translation). | ☐ | |
| 12 | **Healthcheck** | tech-design §7 | `GET /healthz` over tailnet | `200 {"status":"ok"}`. | ☐ | |

## B. iPad / PWA scenarios (on-device only)

| # | Scenario | Story | Steps | Expected | Result | Notes |
|---|----------|-------|-------|----------|--------|-------|
| 13 | **No layout artifacts, portrait + landscape, light + dark** | CH-20 | Rotate; toggle iOS dark mode on every screen | No visual artifacts; Nordic Kitchen dark palette correct; all touch targets ≥ 44×44 pt; AA contrast holds. | ☐ | |
| 14 | **PWA install + offline cache** | CH-6 / CH-20 | "Add to Home Screen"; launch standalone; enable Airplane mode | Installs with iOS web-app chrome; **Service Worker registers** (needs HTTPS); offline: a cached recipe loads on **full reload AND** on **HTMX tap-through** (verifies the split-cache fix). | ☐ | |
| 15 | **No non-standard gestures** | CH-20 | Use the app normally | No scroll-jacking, no custom swipe hijacking; native scroll/zoom behave. | ☐ | |

---

## C. Success-metric capture (PRD §11) — fill weekly

The MVP is "successful" if the family uses it **≥ 3 weeks straight**, cooking **≥ 1 recipe/week**
with positive feedback. Capture per week:

| Week | Time open→list (target < 2 min) | Gen time (≤ 30 s) | Cooked ≥1 recipe? | Weeks w/ ≥1 unswapped recipe (target ≥ 60%) | Positive cook-feedback % (target > 70% by wk 4) | Generated a new week? (retention) |
|------|-------------------------------|-------------------|-------------------|---------------------------------------------|------------------------------------------------|-----------------------------------|
| 1 | | | | | | |
| 2 | | | | | | |
| 3 | | | | | | |
| 4 | | | | | | |

**UX goals (PRD §11) — confirm by observation:**
- [ ] Generating a week takes **≤ 2 taps**
- [ ] Cooking view needs **no zoom** on iPad
- [ ] Shopping list used **without edits in > 80%** of weeks

---

## D. Bug log

Severity: **P0** = blocks core flow / data loss · **P1** = major, no clean workaround ·
**P2** = minor / cosmetic. **All P0/P1 must be closed before sign-off (issue #21).**

| ID | Scenario # | Severity | Description | Status |
|----|-----------|----------|-------------|--------|
| | | | | |

---

## Sign-off

- [ ] All functional scenarios (A) pass on-device
- [ ] All iPad/PWA scenarios (B) pass on-device
- [ ] 2 weeks of metrics (C) captured
- [ ] Time open→shopping list confirmed **< 2 min**
- [ ] All **P0/P1** bugs closed
- [ ] Results written up in `.agents/reports/beta-1.md`
