/*
  This file contains small browser-side behavior for the admin UI.
  The server does most rendering and business logic in Go; JavaScript here is
  intentionally minimal.

  Core project behavior in this script:
  - Listen to an HTMX lifecycle event before each request.
  - Read CSRF token from a <meta> tag.
  - Attach that token to request headers so state-changing requests are accepted.

  JavaScript concepts used below:
  - `document` is the web page object.
  - `addEventListener` subscribes to events.
  - A callback function runs when the event fires.
  - Guard clauses (`if (...) return;`) keep code easy to read by exiting early.
*/
document.body.addEventListener("htmx:configRequest", function (event) {
  /*
    Query the first matching element:
    - CSS selector: meta[name="csrf-token"]
    - Returns the element or `null` when not found.

    `var` is function-scoped (older JavaScript style). In modern code, `const`
    or `let` is often preferred, but we keep current style to match file
    conventions.
  */
  var meta = document.querySelector('meta[name="csrf-token"]');

  // If no token element exists, do nothing for this request.
  if (!meta) {
    return;
  }

  /*
    `getAttribute("content")` reads the token from:
      <meta name="csrf-token" content="...">

    If the attribute is missing/empty, bail out.
  */
  var token = meta.getAttribute("content");
  if (!token) {
    return;
  }

  /*
    HTMX provides request details on `event.detail`.
    We mutate outgoing request headers by setting:
      X-CSRF-Token: <token>

    On the server, CSRF middleware validates this header.
  */
  event.detail.headers["X-CSRF-Token"] = token;
});

/*
  THEME MANAGEMENT OVERVIEW:
  - The default mode is "system", which means CSS `prefers-color-scheme` decides.
  - Manual overrides ("light" / "dark") are stored in localStorage so the user's
    preference persists between page loads.
  - We keep state on `<html data-theme="...">`:
      - no attribute => system mode
      - "light"      => forced light
      - "dark"       => forced dark
*/

/*
  `THEME_STORAGE_KEY` is a constant string used for localStorage lookup.
  Using one key keeps read/write behavior consistent across all pages.
*/
var THEME_STORAGE_KEY = "cms_theme_preference";

/*
  Valid theme modes in this application.
  JavaScript arrays are ordered lists; we reuse this list when cycling modes.
*/
var THEME_MODES = ["system", "light", "dark"];

/*
  Read the saved preference from localStorage and sanitize it.
  localStorage can contain stale/invalid values (manual edits/dev tools), so we
  always validate before trusting the value.
*/
function getStoredThemeMode() {
  var stored = localStorage.getItem(THEME_STORAGE_KEY);
  if (THEME_MODES.indexOf(stored) === -1) {
    return "system";
  }
  return stored;
}

/*
  Persist a valid mode to localStorage.
  Guard clauses prevent writing unexpected values and keep state predictable.
*/
function setStoredThemeMode(mode) {
  if (THEME_MODES.indexOf(mode) === -1) {
    return;
  }
  localStorage.setItem(THEME_STORAGE_KEY, mode);
}

/*
  Apply the current mode to the <html> element by mutating `data-theme`.
  In system mode we remove the attribute so CSS media queries control the theme.
*/
function applyThemeMode(mode) {
  var root = document.documentElement;
  if (mode === "system") {
    root.removeAttribute("data-theme");
    return;
  }
  root.setAttribute("data-theme", mode);
}

/*
  Convert a mode value to user-facing label text.
*/
function labelForThemeMode(mode) {
  if (mode === "light") {
    return "Light";
  }
  if (mode === "dark") {
    return "Dark";
  }
  return "System";
}

/*
  Update visible text in any elements that carry `[data-theme-label]`.
  Keeping this in one function ensures the UI always reflects the real mode.
*/
function updateThemeLabels(mode) {
  var labels = document.querySelectorAll("[data-theme-label]");
  var value = labelForThemeMode(mode);
  labels.forEach(function (element) {
    element.textContent = value;
  });
}

/*
  Cycle through `system -> light -> dark -> system`.
*/
function nextThemeMode(currentMode) {
  var currentIndex = THEME_MODES.indexOf(currentMode);
  var safeIndex = currentIndex === -1 ? 0 : currentIndex;
  var nextIndex = (safeIndex + 1) % THEME_MODES.length;
  return THEME_MODES[nextIndex];
}

/*
  Apply + persist + reflect UI in one place to avoid split-brain state.
*/
function commitThemeMode(mode) {
  setStoredThemeMode(mode);
  applyThemeMode(mode);
  updateThemeLabels(mode);
}

/*
  Initialize theme controls on page load.
  - Reads saved mode.
  - Applies the mode.
  - Wires click handlers for all toggle controls.
*/
function initializeThemeControls() {
  var initialMode = getStoredThemeMode();
  applyThemeMode(initialMode);
  updateThemeLabels(initialMode);

  var toggles = document.querySelectorAll("[data-theme-toggle]");
  toggles.forEach(function (toggle) {
    toggle.addEventListener("click", function () {
      var currentMode = getStoredThemeMode();
      var mode = nextThemeMode(currentMode);
      commitThemeMode(mode);
    });
  });
}

/*
  React to OS preference changes when mode is set to "system".
  No attribute changes are needed in this case because CSS media queries handle
  the effective colors; we only refresh labels to keep UI indicators accurate.
*/
function initializeSystemThemeListener() {
  var mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  var listener = function () {
    if (getStoredThemeMode() === "system") {
      updateThemeLabels("system");
    }
  };

  if (typeof mediaQuery.addEventListener === "function") {
    mediaQuery.addEventListener("change", listener);
    return;
  }

  if (typeof mediaQuery.addListener === "function") {
    mediaQuery.addListener(listener);
  }
}

initializeThemeControls();
initializeSystemThemeListener();
