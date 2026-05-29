// "Show purchased" filter for the shopping list (CH-13 / F-3).
// By default the list hides checked-off (purchased) items so the household sees
// only what is left to buy; toggling the control adds a class that reveals them
// again (the CSS keys off .shopping-list--show-purchased). The toggle is a pure
// view preference — item state lives on the server — so nothing is persisted.
// The IIFE degrades to a no-op when the list or toggle is absent (e.g. the empty
// state) and is safe to re-run after an HTMX swap of an item row.
(function () {
  var list = document.querySelector('[data-shopping-list]');
  var toggle = document.querySelector('[data-shopping-filter]');
  if (!list || !toggle) return;

  function apply() {
    if (toggle.checked) list.classList.add('shopping-list--show-purchased');
    else list.classList.remove('shopping-list--show-purchased');
  }

  toggle.addEventListener('change', apply);
  apply();
})();
