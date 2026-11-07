const authView = document.querySelector("#auth-view");
const setupView = document.querySelector("#setup-view");
const authForm = document.querySelector("#account-form");
const authNotice = document.querySelector("#auth-notice");
const setupNotice = document.querySelector("#setup-notice");
const accountBar = document.querySelector("#account-bar");
const accountName = document.querySelector("#account-name");
const state = {
  accessToken: "",
  expiresAt: 0,
  account: null,
  organization: null,
  project: null,
  environment: null,
  key: "",
  step: "organization",
};

const errorMessages = {
  invalid_argument: "Check the fields and try again.",
  invalid_credentials: "That email and password combination was not recognized.",
  email_already_registered: "An account with that email already exists. Try signing in.",
  slug_taken: "That slug is already in use. Choose another one.",
  forbidden: "You don’t have permission to make that change.",
  unauthenticated: "Your session expired. Sign in again to continue.",
  not_found: "We couldn’t find that resource. Sign in again and retry.",
  already_exists: "That environment already exists. Choose a different one.",
  rate_limited: "Too many attempts. Wait a little and try again.",
};

function showNotice(element, message) {
  element.textContent = message;
  element.hidden = false;
}

function clearNotice(element) {
  element.textContent = "";
  element.hidden = true;
}

async function api(path, { method = "POST", body, authenticated = false } = {}) {
  const headers = { Accept: "application/json" };
  if (body) headers["Content-Type"] = "application/json";
  if (authenticated) {
    if (!state.accessToken) throw new Error("Please sign in again to continue.");
    headers.Authorization = `Bearer ${state.accessToken}`;
  }

  let response;
  try {
    response = await fetch(path, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new Error("Couldn’t reach AuthForge. Check your connection and try again.");
  }

  let result;
  try {
    result = await response.json();
  } catch {
    throw new Error("AuthForge returned an unreadable response. Please try again.");
  }
  if (!response.ok) {
    const code = typeof result.error === "string" ? result.error : "internal";
    throw new Error(errorMessages[code] ?? `Something went wrong (${code}). Please try again.`);
  }
  if (typeof result !== "object" || result === null || Array.isArray(result)) {
    throw new Error("AuthForge returned an unexpected response. Please try again.");
  }
  return result;
}

function setBusy(form, busy, label) {
  const button = form.querySelector('button[type="submit"]');
  if (!button) return;
  button.disabled = busy;
  if (busy) {
    button.dataset.originalLabel = button.textContent;
    button.textContent = label;
  } else if (button.dataset.originalLabel) {
    button.textContent = button.dataset.originalLabel;
    delete button.dataset.originalLabel;
  }
}

function saveSession() {
  try {
    sessionStorage.setItem("authforge-console", JSON.stringify({
      accessToken: state.accessToken,
      expiresAt: state.expiresAt,
      account: state.account,
      organization: state.organization,
      project: state.project,
      environment: state.environment,
      step: state.step,
    }));
  } catch {
    const target = state.account ? setupNotice : authNotice;
    showNotice(target, "Your browser couldn’t save this session. Keep this tab open until setup is complete.");
  }
}

function restoreSession() {
  try {
    const raw = sessionStorage.getItem("authforge-console");
    if (!raw) return;
    const saved = JSON.parse(raw);
    if (
      typeof saved.accessToken !== "string" ||
      typeof saved.expiresAt !== "number" ||
      saved.expiresAt <= Date.now() ||
      typeof saved.account?.email !== "string" ||
      !["organization", "project", "environment", "key", "complete"].includes(saved.step) ||
      (saved.step !== "organization" && typeof saved.organization?.id !== "string") ||
      (["environment", "key", "complete"].includes(saved.step) && typeof saved.project?.id !== "string") ||
      (["key", "complete"].includes(saved.step) && typeof saved.environment?.id !== "string")
    ) {
      sessionStorage.removeItem("authforge-console");
      return;
    }
    state.accessToken = saved.accessToken;
    state.expiresAt = saved.expiresAt;
    state.account = saved.account;
    state.organization = saved.organization ?? null;
    state.project = saved.project ?? null;
    state.environment = saved.environment ?? null;
    state.step = saved.step;
  } catch {
    sessionStorage.removeItem("authforge-console");
  }
}

function switchAuthMode(mode) {
  const signup = mode === "signup";
  document.querySelector("#signup-tab").classList.toggle("is-active", signup);
  document.querySelector("#signup-tab").setAttribute("aria-selected", String(signup));
  document.querySelector("#login-tab").classList.toggle("is-active", !signup);
  document.querySelector("#login-tab").setAttribute("aria-selected", String(!signup));
  document.querySelector("#name-field").hidden = !signup;
  authForm.elements.name.required = signup;
  authForm.elements.password.autocomplete = signup ? "new-password" : "current-password";
  document.querySelector("#auth-title").textContent = signup ? "Create your account" : "Welcome back";
  document.querySelector("#auth-description").textContent = signup
    ? "Start building with AuthForge in just a few steps."
    : "Sign in to set up a new workspace.";
  document.querySelector("#account-submit").innerHTML = signup
    ? '<span>Create account</span><span class="button-arrow" aria-hidden="true">→</span>'
    : '<span>Sign in</span><span class="button-arrow" aria-hidden="true">→</span>';
  authForm.dataset.mode = mode;
  clearNotice(authNotice);
}

function renderStep() {
  const complete = state.step === "complete";
  authView.hidden = true;
  setupView.hidden = false;
  accountBar.hidden = false;
  accountName.textContent = state.account?.name || state.account?.email || "";
  document.querySelectorAll(".step-form").forEach((form) => {
    form.hidden = form.dataset.step !== state.step;
  });
  document.querySelector("#complete-panel").hidden = !complete;
  document.querySelector("#setup-title").textContent = complete
    ? "Your workspace is ready."
    : "Let’s set up your workspace.";
  document.querySelector("#setup-description").textContent = complete
    ? "Your application is ready to connect to AuthForge."
    : "Create the building blocks your application needs.";
  document.querySelector("#step-counter").textContent = complete
    ? "COMPLETE"
    : `STEP ${["organization", "project", "environment", "key"].indexOf(state.step) + 1} OF 4`;
  document.querySelectorAll("[data-nav-step]").forEach((item) => {
    const order = ["organization", "project", "environment", "key"];
    const currentIndex = order.indexOf(state.step);
    const itemIndex = order.indexOf(item.dataset.navStep);
    item.classList.toggle("is-active", !complete && itemIndex === currentIndex);
    item.classList.toggle("is-complete", complete || itemIndex < currentIndex);
  });
  if (complete) {
    document.querySelector("#issued-key").textContent = state.key || "This key is no longer available in this tab.";
    document.querySelector("#summary-project").textContent = state.project?.name ?? "";
    document.querySelector("#summary-environment").textContent = state.environment?.name ?? "";
  }
}

function handleError(element, error) {
  showNotice(element, error instanceof Error ? error.message : "Something went wrong. Please try again.");
}

document.querySelector("#signup-tab").addEventListener("click", () => switchAuthMode("signup"));
document.querySelector("#login-tab").addEventListener("click", () => switchAuthMode("login"));

authForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearNotice(authNotice);
  const formData = new FormData(authForm);
  const signup = authForm.dataset.mode !== "login";
  const body = {
    email: String(formData.get("email") ?? "").trim(),
    password: String(formData.get("password") ?? ""),
  };
  if (signup) body.name = String(formData.get("name") ?? "").trim();
  setBusy(authForm, true, signup ? "Creating account…" : "Signing in…");
  try {
    const result = await api(signup ? "/v1/console/signup" : "/v1/console/login", { body });
    if (typeof result.access_token !== "string" || typeof result.account?.email !== "string") {
      throw new Error("AuthForge returned an unexpected response. Please try again.");
    }
    state.accessToken = result.access_token;
    state.expiresAt = typeof result.expires_at === "number" ? result.expires_at * 1000 : 0;
    if (!state.expiresAt || state.expiresAt <= Date.now()) {
      throw new Error("AuthForge returned an expired console session. Please try again.");
    }
    state.account = result.account;
    state.organization = null;
    state.project = null;
    state.environment = null;
    state.key = "";
    state.step = "organization";
    saveSession();
    clearNotice(setupNotice);
    renderStep();
  } catch (error) {
    handleError(authNotice, error);
  } finally {
    setBusy(authForm, false);
  }
});

document.querySelector("#organization-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  clearNotice(setupNotice);
  const form = event.currentTarget;
  const values = new FormData(form);
  setBusy(form, true, "Creating organization…");
  try {
    state.organization = await api("/v1/organizations", {
      body: { name: String(values.get("name")).trim(), slug: String(values.get("slug")).trim() },
      authenticated: true,
    });
    state.step = "project";
    saveSession();
    renderStep();
  } catch (error) {
    handleError(setupNotice, error);
  } finally {
    setBusy(form, false);
  }
});

document.querySelector("#project-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  clearNotice(setupNotice);
  const form = event.currentTarget;
  const values = new FormData(form);
  setBusy(form, true, "Creating project…");
  try {
    state.project = await api("/v1/projects", {
      body: {
        organization_id: state.organization.id,
        name: String(values.get("name")).trim(),
        slug: String(values.get("slug")).trim(),
      },
      authenticated: true,
    });
    state.step = "environment";
    saveSession();
    renderStep();
  } catch (error) {
    handleError(setupNotice, error);
  } finally {
    setBusy(form, false);
  }
});

document.querySelector("#environment-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  clearNotice(setupNotice);
  const form = event.currentTarget;
  const values = new FormData(form);
  setBusy(form, true, "Creating environment…");
  try {
    state.environment = await api("/v1/environments", {
      body: { project_id: state.project.id, name: String(values.get("name")) },
      authenticated: true,
    });
    state.step = "key";
    saveSession();
    renderStep();
  } catch (error) {
    handleError(setupNotice, error);
  } finally {
    setBusy(form, false);
  }
});

document.querySelector("#key-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  clearNotice(setupNotice);
  const form = event.currentTarget;
  setBusy(form, true, "Issuing key…");
  try {
    const result = await api("/v1/api-keys", {
      body: { environment_id: state.environment.id, key_class: "publishable" },
      authenticated: true,
    });
    if (typeof result.key !== "string" || result.key.length === 0) {
      throw new Error("AuthForge did not return an API key. Please contact support.");
    }
    state.key = result.key;
    state.step = "complete";
    saveSession();
    renderStep();
  } catch (error) {
    handleError(setupNotice, error);
  } finally {
    setBusy(form, false);
  }
});

document.querySelector("#copy-key").addEventListener("click", async () => {
  const notice = document.querySelector("#copy-notice");
  clearNotice(notice);
  if (!state.key) {
    showNotice(notice, "The key is unavailable. Sign in and create a new workspace key.");
    return;
  }
  try {
    await navigator.clipboard.writeText(state.key);
    notice.textContent = "Copied to clipboard.";
    notice.hidden = false;
  } catch {
    const range = document.createRange();
    range.selectNodeContents(document.querySelector("#issued-key"));
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    notice.textContent = "Select the highlighted key and copy it.";
    notice.hidden = false;
  }
});

document.querySelector("#new-workspace").addEventListener("click", () => {
  state.organization = null;
  state.project = null;
  state.environment = null;
  state.key = "";
  state.step = "organization";
  saveSession();
  clearNotice(setupNotice);
  renderStep();
});

document.querySelector("#logout-button").addEventListener("click", () => {
  sessionStorage.removeItem("authforge-console");
  state.accessToken = "";
  state.expiresAt = 0;
  state.account = null;
  state.organization = null;
  state.project = null;
  state.environment = null;
  state.key = "";
  state.step = "organization";
  accountBar.hidden = true;
  authView.hidden = false;
  setupView.hidden = true;
  authForm.reset();
  switchAuthMode("login");
});

restoreSession();
if (state.accessToken && state.account) renderStep();
else switchAuthMode("signup");
