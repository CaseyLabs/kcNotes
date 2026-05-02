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

function csrfToken() {
  var meta = document.querySelector('meta[name="csrf-token"]');
  if (!meta) {
    return "";
  }
  return meta.getAttribute("content") || "";
}

function base64URLToBuffer(value) {
  var base64 = value.replace(/-/g, "+").replace(/_/g, "/");
  while (base64.length % 4) {
    base64 += "=";
  }
  var binary = atob(base64);
  var bytes = new Uint8Array(binary.length);
  for (var i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

function bufferToBase64URL(value) {
  var bytes = new Uint8Array(value);
  var binary = "";
  for (var i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/g, "");
}

function prepareCreationOptions(options) {
  options.challenge = base64URLToBuffer(options.challenge);
  options.user.id = base64URLToBuffer(options.user.id);
  if (options.excludeCredentials) {
    options.excludeCredentials.forEach(function (credential) {
      credential.id = base64URLToBuffer(credential.id);
    });
  }
  return options;
}

function prepareRequestOptions(options) {
  options.challenge = base64URLToBuffer(options.challenge);
  if (options.allowCredentials) {
    options.allowCredentials.forEach(function (credential) {
      credential.id = base64URLToBuffer(credential.id);
    });
  }
  return options;
}

function credentialToJSON(credential) {
  var response = credential.response;
  var json = {
    id: credential.id,
    rawId: bufferToBase64URL(credential.rawId),
    type: credential.type,
    response: {}
  };
  if (response.clientDataJSON) {
    json.response.clientDataJSON = bufferToBase64URL(response.clientDataJSON);
  }
  if (response.attestationObject) {
    json.response.attestationObject = bufferToBase64URL(
      response.attestationObject
    );
  }
  if (response.authenticatorData) {
    json.response.authenticatorData = bufferToBase64URL(
      response.authenticatorData
    );
  }
  if (response.signature) {
    json.response.signature = bufferToBase64URL(response.signature);
  }
  if (response.userHandle) {
    json.response.userHandle = bufferToBase64URL(response.userHandle);
  }
  if (typeof response.getTransports === "function") {
    json.response.transports = response.getTransports();
  }
  if (credential.authenticatorAttachment) {
    json.authenticatorAttachment = credential.authenticatorAttachment;
  }
  return json;
}

function postForm(url, formData) {
  return fetch(url, {
    method: "POST",
    headers: { "X-CSRF-Token": csrfToken() },
    body: formData || new FormData()
  }).then(function (response) {
    return response.json().then(function (data) {
      if (!response.ok) {
        throw new Error(data.error || "Passkey request failed");
      }
      return data;
    });
  });
}

function postCredential(url, credential) {
  return fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken()
    },
    body: JSON.stringify(credentialToJSON(credential))
  }).then(function (response) {
    return response.json().then(function (data) {
      if (!response.ok) {
        throw new Error(data.error || "Passkey request failed");
      }
      return data;
    });
  });
}

function redirectFrom(data, fallback) {
  window.location.href = data.redirect || fallback;
}

function showPasskeyError(message) {
  var flash = document.querySelector("#flash");
  if (flash) {
    flash.className = "flash-error mb-4";
    flash.textContent = message;
  } else {
    alert(message);
  }
}

function copyText(value) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    return navigator.clipboard.writeText(value);
  }

  return new Promise(function (resolve, reject) {
    var input = document.createElement("textarea");
    input.value = value;
    input.setAttribute("readonly", "readonly");
    input.style.position = "fixed";
    input.style.left = "-9999px";
    document.body.appendChild(input);
    input.select();

    try {
      if (!document.execCommand("copy")) {
        throw new Error("Copy failed");
      }
      resolve();
    } catch (error) {
      reject(error);
    } finally {
      document.body.removeChild(input);
    }
  });
}

document.body.addEventListener("click", function (event) {
  var button = event.target.closest("[data-copy-value]");
  if (!button) {
    return;
  }

  var value = button.getAttribute("data-copy-value");
  if (!value) {
    return;
  }

  var originalText = button.textContent;
  setBusy(button, true);
  copyText(value)
    .then(function () {
      button.textContent = "Copied";
      window.setTimeout(function () {
        button.textContent = originalText;
      }, 1600);
    })
    .catch(function () {
      showPasskeyError("Copy failed. Select the link and copy it manually.");
    })
    .finally(function () {
      setBusy(button, false);
    });
});

function passkeysAvailable() {
  return !!(window.PublicKeyCredential && navigator.credentials);
}

function setBusy(control, busy) {
  if (!control) {
    return;
  }
  control.disabled = busy;
  control.setAttribute("aria-busy", busy ? "true" : "false");
}

function bindPasskeyLogin() {
  var button = document.querySelector("[data-passkey-login]");
  if (!button) {
    return;
  }
  button.addEventListener("click", function () {
    if (!passkeysAvailable()) {
      showPasskeyError(
        "Passkeys require a compatible browser on HTTPS or localhost."
      );
      return;
    }
    setBusy(button, true);
    postForm("/admin/login/passkeys/start")
      .then(function (data) {
        return navigator.credentials
          .get({ publicKey: prepareRequestOptions(data.publicKey) })
          .then(function (credential) {
            return postCredential(
              "/admin/login/passkeys/finish?challenge_id=" +
                encodeURIComponent(data.challenge_id),
              credential
            );
          });
      })
      .then(function (data) {
        redirectFrom(data, "/admin");
      })
      .catch(function (error) {
        showPasskeyError(error.message);
      })
      .finally(function () {
        setBusy(button, false);
      });
  });
}

function bindPasskeyRegistration(selector, startURL, finishURL, fallback) {
  var form = document.querySelector(selector);
  if (!form) {
    return;
  }
  form.addEventListener("submit", function (event) {
    event.preventDefault();
    if (!passkeysAvailable()) {
      showPasskeyError(
        "Passkeys require a compatible browser on HTTPS or localhost."
      );
      return;
    }
    var submitButton = form.querySelector('button[type="submit"]');
    setBusy(submitButton, true);
    var formData = new FormData(form);
    postForm(startURL, formData)
      .then(function (data) {
        return navigator.credentials
          .create({ publicKey: prepareCreationOptions(data.publicKey) })
          .then(function (credential) {
            var url =
              finishURL +
              "?challenge_id=" +
              encodeURIComponent(data.challenge_id);
            if (data.user_id) {
              url +=
                "&user_id=" +
                encodeURIComponent(data.user_id) +
                "&email=" +
                encodeURIComponent(data.email || "") +
                "&nickname=" +
                encodeURIComponent(data.nickname || "");
            }
            var token = formData.get("token");
            if (token) {
              url += "&token=" + encodeURIComponent(token);
            }
            return postCredential(url, credential);
          });
      })
      .then(function (data) {
        redirectFrom(data, fallback);
      })
      .catch(function (error) {
        showPasskeyError(error.message);
      })
      .finally(function () {
        setBusy(submitButton, false);
      });
  });
}

bindPasskeyLogin();
bindPasskeyRegistration(
  "[data-passkey-setup]",
  "/admin/setup/passkeys/start",
  "/admin/setup/passkeys/finish",
  "/admin"
);
bindPasskeyRegistration(
  "[data-passkey-enroll]",
  "/admin/enroll/passkeys/start",
  "/admin/enroll/passkeys/finish",
  "/admin"
);
bindPasskeyRegistration(
  "[data-passkey-add]",
  "/admin/passkeys/start",
  "/admin/passkeys/finish",
  "/admin/passkeys"
);

/*
  THEME MANAGEMENT OVERVIEW:
  - The default mode is "dark", matching the app's primary admin/public design.
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
    return "dark";
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
