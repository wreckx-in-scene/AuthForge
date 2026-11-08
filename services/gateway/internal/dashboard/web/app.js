const authView = document.querySelector("#auth-view");
const setupView = document.querySelector("#setup-view");
const workspaceView = document.querySelector("#workspace-view");
const authForm = document.querySelector("#account-form");
const authNotice = document.querySelector("#auth-notice");
const setupNotice = document.querySelector("#setup-notice");
const accountBar = document.querySelector("#account-bar");
const accountName = document.querySelector("#account-name");
const resetRequestForm = document.querySelector("#password-reset-request-form");
const resetRequestNotice = document.querySelector("#password-reset-request-notice");
const resetPasswordForm = document.querySelector("#password-reset-form");
const state = {
  accessToken: "",
  expiresAt: 0,
  account: null,
  organization: null,
  project: null,
  environment: null,
  key: "",
  step: "organization",
  selectedOrganizationID: "",
  selectedProjectID: "",
  selectedEnvironmentID: "",
  workspaceOrganizations: [],
  workspaceProjects: [],
  workspaceEnvironments: [],
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
  invalid_redirect_uri: "Use valid HTTPS callback URLs, or localhost for development.",
  invalid_reset_token: "This reset link has expired or was already used. Request a new one.",
  invalid_verification_token: "This verification link has expired or was already used. Request a new one.",
  rate_limited: "Too many attempts. Wait a little and try again.",
};

function showNotice(element, message) {
  element.textContent = message;
  element.hidden = false;
}

function clearNotice(element) {
  element.textContent = "";
  element.hidden = true;
  element.classList.remove("notice-success");
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
      selectedOrganizationID: state.selectedOrganizationID,
      selectedProjectID: state.selectedProjectID,
      selectedEnvironmentID: state.selectedEnvironmentID,
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
      !["organization", "project", "environment", "key", "complete", "workspace"].includes(saved.step) ||
      (!["organization", "workspace"].includes(saved.step) && typeof saved.organization?.id !== "string") ||
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
    state.selectedOrganizationID = saved.selectedOrganizationID ?? "";
    state.selectedProjectID = saved.selectedProjectID ?? "";
    state.selectedEnvironmentID = saved.selectedEnvironmentID ?? "";
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
    : "Sign in to manage your workspaces.";
  document.querySelector("#account-submit").innerHTML = signup
    ? '<span>Create account</span><span class="button-arrow" aria-hidden="true">→</span>'
    : '<span>Sign in</span><span class="button-arrow" aria-hidden="true">→</span>';
  authForm.dataset.mode = mode;
  document.querySelector("#forgot-password").hidden = signup;
  document.querySelector(".mode-switch").hidden = false;
  authForm.hidden = false;
  resetRequestForm.hidden = true;
  clearNotice(resetRequestNotice);
  clearNotice(authNotice);
}

function renderStep() {
  const complete = state.step === "complete";
  authView.hidden = true;
  setupView.hidden = false;
  workspaceView.hidden = true;
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

function addEmptyMessage(container, message) {
  const empty = document.createElement("p");
  empty.className = "resource-empty";
  empty.textContent = message;
  container.replaceChildren(empty);
}

function renderResourceOptions(container, items, selectedID, getLabel, onSelect) {
  container.replaceChildren();
  if (items.length === 0) {
    addEmptyMessage(container, "Nothing here yet.");
    return;
  }
  for (const item of items) {
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.id = item.id;
    button.className = `resource-option${item.id === selectedID ? " is-selected" : ""}`;
    const label = document.createElement("span");
    const title = document.createElement("strong");
    const detail = document.createElement("small");
    title.textContent = item.name;
    detail.textContent = getLabel(item);
    label.append(title, detail);
    const arrow = document.createElement("span");
    arrow.className = "resource-arrow";
    arrow.setAttribute("aria-hidden", "true");
    arrow.textContent = "›";
    button.append(label, arrow);
    button.addEventListener("click", () => onSelect(item.id));
    container.append(button);
  }
}

function markSelected(container, selectedID) {
  container.querySelectorAll(".resource-option").forEach((item) => {
    item.classList.toggle("is-selected", item.dataset.id === selectedID);
  });
}

function renderKeyRows(keys) {
  const container = document.querySelector("#key-list");
  container.replaceChildren();
  if (keys.length === 0) {
    addEmptyMessage(container, "No API keys yet. Issue a publishable key to connect your app.");
    return;
  }
  for (const key of keys) {
    const row = document.createElement("div");
    row.className = "key-row";
    const main = document.createElement("span");
    main.className = "key-row-main";
    const prefix = document.createElement("code");
    prefix.textContent = key.prefix;
    const details = document.createElement("small");
    details.textContent = `${key.key_class} · created ${new Date(key.created_at * 1000).toLocaleDateString()}`;
    main.append(prefix, details);
    const status = document.createElement("span");
    status.className = `key-status${key.status === "revoked" ? " revoked" : ""}`;
    status.textContent = key.status;
    row.append(main, status);
    if (key.status === "active") {
      const revoke = document.createElement("button");
      revoke.type = "button";
      revoke.className = "text-button";
      revoke.textContent = "Revoke";
      revoke.addEventListener("click", () => revokeAPIKey(key.id, key.prefix));
      row.append(revoke);
    } else {
      row.append(document.createElement("span"));
    }
    container.append(row);
  }
}

function renderOAuthClientRows(clients) {
  const container = document.querySelector("#oauth-client-list");
  container.replaceChildren();
  if (clients.length === 0) {
    addEmptyMessage(container, "No OAuth clients registered for this environment.");
    return;
  }
  for (const client of clients) {
    const row = document.createElement("div");
    row.className = "key-row";
    const main = document.createElement("span");
    main.className = "key-row-main";
    const name = document.createElement("strong");
    name.textContent = client.name;
    const details = document.createElement("small");
    details.textContent = `Client ID: ${client.client_id}`;
    const callbacks = document.createElement("small");
    callbacks.textContent = client.redirect_uris.join(" · ");
    main.append(name, details, callbacks);
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "text-button";
    remove.textContent = "Delete";
    remove.addEventListener("click", () => deleteOAuthClient(client.client_id, client.name));
    row.append(main, remove);
    container.append(row);
  }
}

function showWorkspaceView() {
  authView.hidden = true;
  setupView.hidden = true;
  workspaceView.hidden = false;
  accountBar.hidden = false;
  accountName.textContent = state.account?.name || state.account?.email || "";
}

async function openWorkspace() {
  state.step = "workspace";
  state.key = "";
  state.organization = null;
  state.project = null;
  state.environment = null;
  saveSession();
  clearNotice(document.querySelector("#workspace-notice"));
  showWorkspaceView();
  await loadOrganizations();
}

async function loadOrganizations() {
  const notice = document.querySelector("#workspace-notice");
  clearNotice(notice);
  try {
    const response = await api("/v1/organizations", { method: "GET", authenticated: true });
    const organizations = Array.isArray(response.organizations) ? response.organizations : [];
    if (!organizations.every((item) => typeof item.id === "string" && typeof item.name === "string" && typeof item.slug === "string")) {
      throw new Error("AuthForge returned an unexpected organization list.");
    }
    state.workspaceOrganizations = organizations;
    const selected = organizations.find((item) => item.id === state.selectedOrganizationID);
    const organizationID = selected?.id ?? organizations[0]?.id ?? "";
    renderResourceOptions(
      document.querySelector("#organization-list"),
      organizations,
      organizationID,
      (item) => item.slug,
      selectOrganization,
    );
    if (organizationID) {
      await selectOrganization(organizationID);
    } else {
      state.selectedOrganizationID = "";
      state.selectedProjectID = "";
      state.selectedEnvironmentID = "";
      state.organization = null;
      state.project = null;
      state.environment = null;
      state.workspaceProjects = [];
      state.workspaceEnvironments = [];
      renderResourceOptions(document.querySelector("#project-list"), [], "", () => "", () => {});
      addEmptyMessage(document.querySelector("#project-list"), "Create an organization to get started.");
      addEmptyMessage(document.querySelector("#environment-list"), "Choose a project.");
      addEmptyMessage(document.querySelector("#key-list"), "Choose an environment.");
      addEmptyMessage(document.querySelector("#oauth-client-list"), "Choose an environment.");
      document.querySelector("#add-project").disabled = true;
      document.querySelector("#add-environment").disabled = true;
      document.querySelector("#issue-key").disabled = true;
      document.querySelector("#create-oauth-client").disabled = true;
    }
  } catch (error) {
    handleError(notice, error);
  }
}

async function selectOrganization(organizationID) {
  const notice = document.querySelector("#workspace-notice");
  const previousProjectID = state.selectedProjectID;
  state.organization = state.workspaceOrganizations.find((item) => item.id === organizationID) ?? null;
  state.selectedOrganizationID = organizationID;
  state.selectedProjectID = "";
  state.selectedEnvironmentID = "";
  state.project = null;
  state.environment = null;
  state.workspaceProjects = [];
  state.workspaceEnvironments = [];
  markSelected(document.querySelector("#organization-list"), organizationID);
  saveSession();
  document.querySelector("#add-project").disabled = false;
  document.querySelector("#add-environment").disabled = true;
  document.querySelector("#issue-key").disabled = true;
  document.querySelector("#create-oauth-client").disabled = true;
  document.querySelector("#new-key-notice").hidden = true;
  try {
    const response = await api(`/v1/organizations/${encodeURIComponent(organizationID)}/projects`, {
      method: "GET",
      authenticated: true,
    });
    const projects = Array.isArray(response.projects) ? response.projects : [];
    if (!projects.every((item) => typeof item.id === "string" && typeof item.name === "string")) {
      throw new Error("AuthForge returned an unexpected project list.");
    }
    state.workspaceProjects = projects;
    const selected = projects.find((item) => item.id === previousProjectID);
    const projectID = selected?.id ?? projects[0]?.id ?? "";
    renderResourceOptions(
      document.querySelector("#project-list"),
      projects,
      projectID,
      (item) => item.slug,
      selectProject,
    );
    if (projectID) await selectProject(projectID);
    else {
      state.project = null;
      state.environment = null;
      state.workspaceEnvironments = [];
      addEmptyMessage(document.querySelector("#environment-list"), "Choose a project.");
      addEmptyMessage(document.querySelector("#key-list"), "Choose an environment.");
      addEmptyMessage(document.querySelector("#oauth-client-list"), "Choose an environment.");
    }
  } catch (error) {
    handleError(notice, error);
  }
}

async function selectProject(projectID) {
  const notice = document.querySelector("#workspace-notice");
  const previousEnvironmentID = state.selectedEnvironmentID;
  state.project = state.workspaceProjects.find((item) => item.id === projectID) ?? null;
  state.selectedProjectID = projectID;
  state.selectedEnvironmentID = "";
  state.environment = null;
  state.workspaceEnvironments = [];
  markSelected(document.querySelector("#project-list"), projectID);
  saveSession();
  document.querySelector("#add-environment").disabled = false;
  document.querySelector("#issue-key").disabled = true;
  document.querySelector("#create-oauth-client").disabled = true;
  document.querySelector("#new-key-notice").hidden = true;
  try {
    const response = await api(`/v1/projects/${encodeURIComponent(projectID)}/environments`, {
      method: "GET",
      authenticated: true,
    });
    const environments = Array.isArray(response.environments) ? response.environments : [];
    if (!environments.every((item) => typeof item.id === "string" && typeof item.name === "string")) {
      throw new Error("AuthForge returned an unexpected environment list.");
    }
    state.workspaceEnvironments = environments;
    const selected = environments.find((item) => item.id === previousEnvironmentID);
    const environmentID = selected?.id ?? environments[0]?.id ?? "";
    renderResourceOptions(
      document.querySelector("#environment-list"),
      environments,
      environmentID,
      () => "Environment",
      selectEnvironment,
    );
    if (environmentID) await selectEnvironment(environmentID);
    else {
      state.environment = null;
      addEmptyMessage(document.querySelector("#key-list"), "Choose an environment.");
      addEmptyMessage(document.querySelector("#oauth-client-list"), "Choose an environment.");
    }
  } catch (error) {
    handleError(notice, error);
  }
}

async function selectEnvironment(environmentID) {
  const notice = document.querySelector("#workspace-notice");
  state.environment = state.workspaceEnvironments.find((item) => item.id === environmentID) ?? null;
  state.selectedEnvironmentID = environmentID;
  markSelected(document.querySelector("#environment-list"), environmentID);
  document.querySelector("#new-key-notice").hidden = true;
  saveSession();
  document.querySelector("#issue-key").disabled = false;
  document.querySelector("#create-oauth-client").disabled = false;
  try {
    const [keyResponse, clientResponse] = await Promise.all([
      api(`/v1/environments/${encodeURIComponent(environmentID)}/api-keys`, {
        method: "GET",
        authenticated: true,
      }),
      api(`/v1/environments/${encodeURIComponent(environmentID)}/oauth-clients`, {
        method: "GET",
        authenticated: true,
      }),
    ]);
    const keys = Array.isArray(keyResponse.keys) ? keyResponse.keys : [];
    if (!keys.every((item) => typeof item.id === "string" && typeof item.prefix === "string" && typeof item.status === "string")) {
      throw new Error("AuthForge returned an unexpected API key list.");
    }
    const clients = Array.isArray(clientResponse.clients) ? clientResponse.clients : [];
    if (!clients.every((item) =>
      typeof item.client_id === "string" &&
      typeof item.name === "string" &&
      Array.isArray(item.redirect_uris) &&
      item.redirect_uris.every((uri) => typeof uri === "string")
    )) {
      throw new Error("AuthForge returned an unexpected OAuth client list.");
    }
    renderKeyRows(keys);
    renderOAuthClientRows(clients);
  } catch (error) {
    handleError(notice, error);
  }
}

async function revokeAPIKey(keyID, prefix) {
  if (!window.confirm(`Revoke API key ${prefix}? Applications using it will no longer be able to authenticate.`)) return;
  const notice = document.querySelector("#workspace-notice");
  try {
    await api(`/v1/api-keys/${encodeURIComponent(keyID)}`, {
      method: "DELETE",
      authenticated: true,
    });
    await selectEnvironment(state.selectedEnvironmentID);
  } catch (error) {
    handleError(notice, error);
  }
}

async function deleteOAuthClient(clientID, name) {
  if (!window.confirm(`Delete OAuth client “${name}”? Sign-in requests for this client will stop working.`)) return;
  const notice = document.querySelector("#workspace-notice");
  try {
    await api(`/v1/oauth-clients/${encodeURIComponent(clientID)}`, {
      method: "DELETE",
      authenticated: true,
    });
    await selectEnvironment(state.selectedEnvironmentID);
  } catch (error) {
    handleError(notice, error);
  }
}

function startSetup(step, organization = null, project = null) {
  state.organization = organization;
  state.project = project;
  state.environment = null;
  state.key = "";
  state.step = step;
  saveSession();
  clearNotice(setupNotice);
  renderStep();
}

function handleError(element, error) {
  showNotice(element, error instanceof Error ? error.message : "Something went wrong. Please try again.");
}

document.querySelector("#signup-tab").addEventListener("click", () => switchAuthMode("signup"));
document.querySelector("#login-tab").addEventListener("click", () => switchAuthMode("login"));

document.querySelector("#forgot-password").addEventListener("click", () => {
  const email = authForm.elements.email.value.trim();
  document.querySelector(".mode-switch").hidden = true;
  authForm.hidden = true;
  document.querySelector("#forgot-password").hidden = true;
  resetRequestForm.hidden = false;
  resetRequestForm.elements.email.value = email;
  document.querySelector("#auth-title").textContent = "Reset your password";
  document.querySelector("#auth-description").textContent = "We’ll email you a secure link if an account exists for that address.";
  clearNotice(authNotice);
  clearNotice(resetRequestNotice);
});

document.querySelector("#back-to-login").addEventListener("click", () => {
  authForm.elements.email.value = resetRequestForm.elements.email.value;
  switchAuthMode("login");
});

resetRequestForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearNotice(resetRequestNotice);
  setBusy(resetRequestForm, true, "Sending reset link…");
  try {
    await api("/v1/password-reset/request", {
      body: { email: String(new FormData(resetRequestForm).get("email")).trim() },
    });
    showNotice(resetRequestNotice, "If an account exists for that address, a password reset link has been sent.");
  } catch (error) {
    handleError(resetRequestNotice, error);
  } finally {
    setBusy(resetRequestForm, false);
  }
});

resetPasswordForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const notice = document.querySelector("#password-reset-notice");
  clearNotice(notice);
  const values = new FormData(resetPasswordForm);
  const password = String(values.get("password"));
  if (password !== String(values.get("confirm-password"))) {
    showNotice(notice, "The passwords don’t match.");
    return;
  }
  const token = new URLSearchParams(window.location.search).get("token");
  if (!token) {
    showNotice(notice, "This reset link is missing its token. Request a new password reset link.");
    return;
  }
  setBusy(resetPasswordForm, true, "Resetting password…");
  try {
    const result = await api("/v1/password-reset/confirm", {
      body: { reset_token: token, password },
    });
    if (result.ok !== true) throw new Error("AuthForge could not reset the password. Request a new link and try again.");
    window.history.replaceState(null, "", "/reset-password");
    sessionStorage.removeItem("authforge-console");
    resetPasswordForm.reset();
    resetPasswordForm.hidden = true;
    document.querySelector("#reset-sign-in").textContent = "Sign in with your new password";
    showNotice(notice, "Your password has been reset. You can now sign in with your new password.");
    notice.classList.add("notice-success");
    document.querySelector("#reset-sign-in").focus();
  } catch (error) {
    handleError(notice, error);
  } finally {
    setBusy(resetPasswordForm, false);
  }
});

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
    state.selectedOrganizationID = "";
    state.selectedProjectID = "";
    state.selectedEnvironmentID = "";
    if (signup) {
      state.step = "organization";
      saveSession();
      clearNotice(setupNotice);
      renderStep();
    } else {
      await openWorkspace();
    }
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
    state.selectedOrganizationID = state.organization.id;
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
    state.selectedProjectID = state.project.id;
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
    state.selectedEnvironmentID = state.environment.id;
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
  openWorkspace();
});

document.querySelector("#refresh-workspaces").addEventListener("click", loadOrganizations);
document.querySelector("#add-organization").addEventListener("click", () => startSetup("organization"));
document.querySelector("#add-project").addEventListener("click", () => {
  if (state.organization) startSetup("project", state.organization);
});
document.querySelector("#add-environment").addEventListener("click", () => {
  if (state.organization && state.project) startSetup("environment", state.organization, state.project);
});

document.querySelector("#issue-key").addEventListener("click", async () => {
  const notice = document.querySelector("#workspace-notice");
  clearNotice(notice);
  try {
    const response = await api("/v1/api-keys", {
      body: { environment_id: state.selectedEnvironmentID, key_class: "publishable" },
      authenticated: true,
    });
    if (typeof response.key !== "string" || response.key.length === 0) {
      throw new Error("AuthForge did not return an API key.");
    }
    await selectEnvironment(state.selectedEnvironmentID);
    document.querySelector("#new-key-value").textContent = response.key;
    document.querySelector("#new-key-notice").hidden = false;
  } catch (error) {
    handleError(notice, error);
  }
});

document.querySelector("#oauth-client-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const notice = document.querySelector("#workspace-notice");
  clearNotice(notice);
  const values = new FormData(form);
  const redirectURIs = String(values.get("redirect_uris"))
    .split(/\r?\n/)
    .map((uri) => uri.trim())
    .filter(Boolean);
  setBusy(form, true, "Registering…");
  try {
    await api("/v1/oauth-clients", {
      body: {
        environment_id: state.selectedEnvironmentID,
        name: String(values.get("name")).trim(),
        redirect_uris: redirectURIs,
      },
      authenticated: true,
    });
    form.reset();
    await selectEnvironment(state.selectedEnvironmentID);
    showNotice(notice, "OAuth client registered. Keep its exact callback URL list up to date.");
    notice.classList.add("notice-success");
  } catch (error) {
    handleError(notice, error);
  } finally {
    setBusy(form, false);
  }
});

document.querySelector("#copy-new-key").addEventListener("click", async () => {
  const notice = document.querySelector("#workspace-notice");
  try {
    await navigator.clipboard.writeText(document.querySelector("#new-key-value").textContent);
    showNotice(notice, "New key copied. Store it securely; AuthForge won’t show it again.");
  } catch {
    const range = document.createRange();
    range.selectNodeContents(document.querySelector("#new-key-value"));
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    showNotice(notice, "Select the highlighted key and copy it now.");
  }
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
  state.selectedOrganizationID = "";
  state.selectedProjectID = "";
  state.selectedEnvironmentID = "";
  accountBar.hidden = true;
  authView.hidden = false;
  setupView.hidden = true;
  workspaceView.hidden = true;
  authForm.reset();
  switchAuthMode("login");
});

function setHostedAuthMode(mode) {
  const signup = mode === "signup";
  const loginTab = document.querySelector("#hosted-login-tab");
  const signupTab = document.querySelector("#hosted-signup-tab");
  loginTab.classList.toggle("is-active", !signup);
  loginTab.setAttribute("aria-selected", String(!signup));
  signupTab.classList.toggle("is-active", signup);
  signupTab.setAttribute("aria-selected", String(signup));
  document.querySelector("#hosted-auth-form").elements.action.value = signup ? "signup" : "login";
  document.querySelector("#hosted-auth-form").elements.password.autocomplete = signup ? "new-password" : "current-password";
  document.querySelector("#hosted-auth-submit").textContent = signup ? "Create account" : "Continue";
}

function initializeHostedAuthorization() {
  const form = document.querySelector("#hosted-auth-form");
  const params = new URLSearchParams(window.location.search);
  for (const name of [
    "client_id", "redirect_uri", "response_type", "scope",
    "state", "code_challenge", "code_challenge_method",
  ]) {
    const value = params.get(name);
    if (!value) {
      showNotice(document.querySelector("#hosted-auth-notice"), "This sign-in request is incomplete. Return to your application and try again.");
      form.hidden = true;
      return;
    }
    form.elements[name].value = value;
  }
  form.elements.nonce.value = params.get("nonce") ?? "";
  form.elements.nonce.disabled = !params.has("nonce");
  document.querySelector("#hosted-login-tab").addEventListener("click", () => setHostedAuthMode("login"));
  document.querySelector("#hosted-signup-tab").addEventListener("click", () => setHostedAuthMode("signup"));
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const notice = document.querySelector("#hosted-auth-notice");
    clearNotice(notice);
    setBusy(form, true, "Please wait…");
    try {
      const response = await fetch("/v1/authorize", {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/x-www-form-urlencoded",
        },
        body: new URLSearchParams(new FormData(form)),
      });
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
      if (typeof result.redirect_url !== "string") {
        throw new Error("AuthForge returned an unexpected response. Please try again.");
      }
      window.location.assign(result.redirect_url);
    } catch (error) {
      handleError(notice, error);
    } finally {
      setBusy(form, false);
    }
  });
}

const demoSession = {
  email: "",
  tokens: null,
};

function demoNotice(message, success = false) {
  const notice = document.querySelector("#demo-notice");
  showNotice(notice, message);
  notice.classList.toggle("notice-success", success);
}

function demoKey() {
  const key = document.querySelector("#demo-api-key").value.trim();
  if (!key) throw new Error("Enter a development publishable key from your workspace.");
  if (!key.startsWith("af_test_pk_")) {
    throw new Error("For safety, this demo accepts development publishable keys only.");
  }
  return key;
}

async function demoRequest(path, { method = "POST", body, accessToken, includeKey = true } = {}) {
  const headers = { Accept: "application/json" };
  if (includeKey) headers["X-Api-Key"] = demoKey();
  if (body) headers["Content-Type"] = "application/json";
  if (accessToken) headers.Authorization = ["Bearer", accessToken].join(" ");
  let response;
  try {
    response = await fetch(path, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new Error("Couldn’t reach AuthForge. Check the development stack and try again.");
  }
  let result;
  try {
    result = await response.json();
  } catch {
    throw new Error("AuthForge returned an unreadable response.");
  }
  if (!response.ok) {
    const code = typeof result.error === "string" ? result.error : "request_failed";
    if (code === "rate_limited") throw new Error("Too many attempts. Wait before trying again.");
    throw new Error(`AuthForge request failed (${code}).`);
  }
  if (typeof result !== "object" || result === null || Array.isArray(result)) {
    throw new Error("AuthForge returned an unexpected response.");
  }
  return result;
}

function renderDemoSession() {
  const panel = document.querySelector("#demo-session");
  const status = document.querySelector("#demo-verification-status");
  const tokens = demoSession.tokens;
  panel.hidden = !tokens;
  if (!tokens) return;
  status.textContent = tokens.email_verified ? "Email verified" : "Email not verified";
  status.classList.toggle("revoked", !tokens.email_verified);
  document.querySelector("#demo-resend-verification").hidden = tokens.email_verified;
  document.querySelector("#demo-user").textContent = JSON.stringify({
    email: demoSession.email,
    user_id: tokens.user_id,
    expires_at: new Date(tokens.expires_at * 1000).toISOString(),
    email_verified: tokens.email_verified,
  }, null, 2);
}

async function demoAuthenticate(form, path, label) {
  const notice = document.querySelector("#demo-notice");
  clearNotice(notice);
  const values = new FormData(form);
  const email = String(values.get("email")).trim();
  setBusy(form, true, label);
  try {
    demoSession.tokens = await demoRequest(path, {
      body: { email, password: String(values.get("password")) },
    });
    demoSession.email = email;
    renderDemoSession();
    demoNotice(
      demoSession.tokens.email_verified
        ? "Signed in. Your email is verified."
        : "Signed in. Check your email for the verification link.",
      true,
    );
  } catch (error) {
    handleError(notice, error);
  } finally {
    setBusy(form, false);
  }
}

document.querySelector("#demo-signup-form").addEventListener("submit", (event) => {
  event.preventDefault();
  demoAuthenticate(event.currentTarget, "/v1/signup", "Creating account…");
});

document.querySelector("#demo-login-form").addEventListener("submit", (event) => {
  event.preventDefault();
  demoAuthenticate(event.currentTarget, "/v1/login", "Signing in…");
});

document.querySelector("#demo-resend-verification").addEventListener("click", async () => {
  const notice = document.querySelector("#demo-notice");
  clearNotice(notice);
  try {
    if (!demoSession.tokens) throw new Error("Sign in first to resend a verification email.");
    await demoRequest("/v1/email/verification/request", {
      body: { email: demoSession.email },
    });
    demoNotice("If the account needs verification, a fresh link has been sent.", true);
  } catch (error) {
    handleError(notice, error);
  }
});

document.querySelector("#demo-current-user").addEventListener("click", async () => {
  const notice = document.querySelector("#demo-notice");
  clearNotice(notice);
  try {
    if (!demoSession.tokens) throw new Error("Sign in first to load the current user.");
    const user = await demoRequest("/v1/me", {
      method: "GET",
      includeKey: false,
      accessToken: demoSession.tokens.access_token,
    });
    document.querySelector("#demo-user").textContent = JSON.stringify(user, null, 2);
    demoNotice("Current user loaded.", true);
  } catch (error) {
    handleError(notice, error);
  }
});

document.querySelector("#demo-refresh").addEventListener("click", async () => {
  const notice = document.querySelector("#demo-notice");
  clearNotice(notice);
  try {
    if (!demoSession.tokens) throw new Error("Sign in first to refresh the session.");
    demoSession.tokens = await demoRequest("/v1/token/refresh", {
      body: { refresh_token: demoSession.tokens.refresh_token },
      includeKey: false,
    });
    renderDemoSession();
    demoNotice("Session refreshed. The previous refresh token can no longer be reused.", true);
  } catch (error) {
    handleError(notice, error);
  }
});

document.querySelector("#demo-logout").addEventListener("click", async () => {
  const notice = document.querySelector("#demo-notice");
  clearNotice(notice);
  try {
    if (!demoSession.tokens) throw new Error("There is no active session to log out.");
    await demoRequest("/v1/logout", {
      body: { refresh_token: demoSession.tokens.refresh_token },
      includeKey: false,
    });
    demoSession.tokens = null;
    demoSession.email = "";
    renderDemoSession();
    demoNotice("You have been logged out.", true);
  } catch (error) {
    handleError(notice, error);
  }
});

restoreSession();
if (window.location.pathname === "/authorize") {
  document.querySelector("#auth-view").hidden = true;
  document.querySelector("#setup-view").hidden = true;
  document.querySelector("#workspace-view").hidden = true;
  document.querySelector("#reset-password-view").hidden = true;
  document.querySelector("#verify-email-view").hidden = true;
  document.querySelector("#demo-view").hidden = true;
  document.querySelector("#hosted-auth-view").hidden = false;
  accountBar.hidden = true;
  initializeHostedAuthorization();
} else if (window.location.pathname === "/reset-password") {
  document.querySelector("#auth-view").hidden = true;
  document.querySelector("#setup-view").hidden = true;
  document.querySelector("#workspace-view").hidden = true;
  document.querySelector("#reset-password-view").hidden = false;
  accountBar.hidden = true;
  if (!new URLSearchParams(window.location.search).get("token")) {
    resetPasswordForm.hidden = true;
    showNotice(
      document.querySelector("#password-reset-notice"),
      "This reset link is missing its token. Request a new password reset link.",
    );
  }
} else if (window.location.pathname === "/verify-email") {
  document.querySelector("#auth-view").hidden = true;
  document.querySelector("#setup-view").hidden = true;
  document.querySelector("#workspace-view").hidden = true;
  document.querySelector("#reset-password-view").hidden = true;
  document.querySelector("#verify-email-view").hidden = false;
  accountBar.hidden = true;
  const params = new URLSearchParams(window.location.search);
  const environmentID = params.get("environment_id");
  const verificationToken = params.get("token");
  const notice = document.querySelector("#verify-email-notice");
  if (!environmentID || !verificationToken) {
    showNotice(notice, "This verification link is incomplete. Request a new verification email from your application.");
  } else {
    api("/v1/email/verification/confirm", {
      body: { environment_id: environmentID, token: verificationToken },
    }).then((result) => {
      if (result.verified !== true) throw new Error("AuthForge could not verify this email.");
      window.history.replaceState(null, "", "/verify-email");
      showNotice(notice, "Your email is verified. Return to your application and sign in again.", true);
      notice.setAttribute("role", "status");
    }).catch((error) => {
      handleError(notice, error);
    });
  }
} else if (window.location.pathname === "/demo") {
  document.querySelector("#auth-view").hidden = true;
  document.querySelector("#setup-view").hidden = true;
  document.querySelector("#workspace-view").hidden = true;
  document.querySelector("#reset-password-view").hidden = true;
  document.querySelector("#verify-email-view").hidden = true;
  document.querySelector("#demo-view").hidden = false;
  accountBar.hidden = true;
} else if (state.accessToken && state.account && state.step === "workspace") {
  showWorkspaceView();
  loadOrganizations();
} else if (state.accessToken && state.account) renderStep();
else switchAuthMode("signup");
