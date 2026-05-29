// Cooking step tracker for the fullscreen recipe view (CH-11 / F-4).
// Tap a step body → it becomes the active step (aria-current="step").
// Tap the "Mark done" button → toggles a done flag on the step, persisted to
// sessionStorage keyed by recipe id so a refresh during the cook keeps state.
// The IIFE is idempotent: rebinding over the same DOM after an HTMX swap is a
// no-op because event listeners attach once per element and paint() re-reads
// state. sessionStorage failures (Safari Private mode) degrade to in-memory.
(function () {
  var list = document.querySelector('.recipe__step-list[data-recipe-id]');
  if (!list) return;

  var recipeID = list.getAttribute('data-recipe-id');
  var storageKey = 'recipe:' + recipeID;
  var done = {};
  try {
    done = JSON.parse(sessionStorage.getItem(storageKey) || '{}') || {};
  } catch (_) {
    done = {};
  }

  var steps = list.querySelectorAll('li.recipe-step');
  var activeIndex = 0;

  function persist() {
    try { sessionStorage.setItem(storageKey, JSON.stringify(done)); } catch (_) {}
  }

  function paint() {
    steps.forEach(function (li, i) {
      if (done[i]) li.setAttribute('data-done', 'true');
      else li.removeAttribute('data-done');

      if (i === activeIndex) li.setAttribute('aria-current', 'step');
      else li.removeAttribute('aria-current');

      var btn = li.querySelector('.recipe-step__toggle');
      if (btn) btn.setAttribute('aria-pressed', done[i] ? 'true' : 'false');
    });
  }

  steps.forEach(function (li, i) {
    li.addEventListener('click', function (e) {
      if (e.target.closest('.recipe-step__toggle')) {
        done[i] = !done[i];
        persist();
      } else {
        activeIndex = i;
      }
      paint();
    });
  });

  paint();
})();
