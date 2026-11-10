import { randomBytes } from "node:crypto";
import { createServer } from "node:http";
import { createOidcClient } from "./oidc.mjs";

const issuer = process.env.AUTHFORGE_ISSUER ?? "http://localhost:8080";
const clientId = process.env.AUTHFORGE_CLIENT_ID;
const redirectUri =
  process.env.AUTHFORGE_REDIRECT_URI ?? "http://localhost:3000/auth/callback";
const port = Number(process.env.PORT ?? "3000");
const authforgeOrigin = new URL(issuer).origin;
const mailpitUrl = process.env.MAILPIT_URL ?? "http://localhost:8025";
const oidc = createOidcClient({ issuer, clientId, redirectUri });
const loginTransactions = new Map();
const sessions = new Map();
const production = process.env.NODE_ENV === "production";

const server = createServer(async (request, response) => {
  try {
    const url = new URL(request.url, `http://${request.headers.host ?? "localhost"}`);
    if (request.method === "GET" && url.pathname === "/") {
      return renderHome(request, response);
    }
    if (request.method === "GET" && url.pathname === "/login") {
      return beginLogin(response);
    }
    if (request.method === "GET" && url.pathname === "/auth/callback") {
      return finishLogin(request, response, url);
    }
    if (request.method === "GET" && url.pathname === "/profile") {
      return renderProfile(request, response);
    }
    if (request.method === "POST" && url.pathname === "/logout") {
      return logout(request, response);
    }
    response.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
    response.end("Not found");
  } catch (error) {
    console.error("Request failed:", error.message);
    response.writeHead(500, { "Content-Type": "text/html; charset=utf-8" });
    response.end(page("Sign-in could not be completed", `
      <p>The AuthForge request failed. Check the server console and your local configuration.</p>
      <p><a href="/">Return to the example app</a></p>
    `));
  }
});

server.listen(port, "127.0.0.1", () => {
  console.log(`AuthForge example app listening at http://localhost:${port}`);
  console.log(`AuthForge issuer: ${issuer}`);
  console.log("This sample keeps transactions and sessions in memory; do not deploy it.");
});

async function renderHome(request, response) {
  const session = findSession(request);
  if (!session) {
    response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    response.end(page("AuthForge OIDC example", `
      <p>This sample app delegates sign-in to AuthForge.</p>
      <p><a class="button" href="/login">Sign in or create an account</a></p>
      <p>New users can select <strong>Create account</strong> on the AuthForge sign-in page.</p>
    `));
    return;
  }
  response.writeHead(303, { Location: "/profile" });
  response.end();
}

async function beginLogin(response) {
  const now = Date.now();
  for (const [state, transaction] of loginTransactions) {
    if (transaction.expiresAt <= now) loginTransactions.delete(state);
  }
  const transaction = await oidc.createAuthorizationRequest();
  loginTransactions.set(transaction.state, {
    ...transaction,
    expiresAt: now + 10 * 60 * 1000,
  });
  response.writeHead(302, {
    Location: transaction.authorizationUrl,
    "Cache-Control": "no-store",
    "Set-Cookie": cookie("af_login_state", transaction.state, 600),
  });
  response.end();
}

async function finishLogin(request, response, callbackUrl) {
  const state = callbackUrl.searchParams.get("state");
  const transaction = state ? loginTransactions.get(state) : undefined;
  response.setHeader("Set-Cookie", cookie("af_login_state", "", 0));
  if (
    !state ||
    cookieValue(request, "af_login_state") !== state ||
    !transaction ||
    transaction.expiresAt <= Date.now()
  ) {
    loginTransactions.delete(state);
    response.writeHead(400, { "Content-Type": "text/plain; charset=utf-8" });
    response.end("The sign-in state is missing, expired, or invalid. Start again from the app.");
    return;
  }
  loginTransactions.delete(state);

  const { tokens, claims } = await oidc.exchangeAuthorizationCode({
    callbackUrl: callbackUrl.toString(),
    transaction,
  });
  const sessionId = randomBytes(32).toString("base64url");
  sessions.set(sessionId, { tokens, claims });
  response.writeHead(303, {
    Location: "/profile",
    "Cache-Control": "no-store",
    "Set-Cookie": [
      cookie("af_login_state", "", 0),
      cookie("af_demo_session", sessionId, 60 * 60),
    ],
  });
  response.end();
}

async function renderProfile(request, response) {
  const sessionId = cookieValue(request, "af_demo_session");
  const session = sessionId ? sessions.get(sessionId) : undefined;
  if (!session) {
    response.writeHead(303, { Location: "/" });
    response.end();
    return;
  }

  if (session.tokens.expires_at <= Math.floor(Date.now() / 1000) + 30) {
    session.tokens = await postAuthForge("/v1/token/refresh", {
      refresh_token: session.tokens.refresh_token,
    });
  }
  const profile = await fetch(`${authforgeOrigin}/v1/me`, {
    headers: { Authorization: `Bearer ${session.tokens.access_token}` },
    redirect: "error",
  });
  const profileBody = await profile.json();
  if (!profile.ok) {
    throw new Error(
      `Authenticated app API request failed (${profile.status}): ${profileBody.error ?? "request_failed"}`,
    );
  }
  response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
  response.end(page("Signed in", `
    <h1>Signed in through AuthForge</h1>
    <dl>
      <dt>Email</dt><dd>${escapeHtml(session.claims.email)}</dd>
      <dt>Email verified</dt><dd>${session.claims.email_verified ? "Yes" : "Not yet"}</dd>
      <dt>AuthForge user ID</dt><dd>${escapeHtml(profileBody.user_id)}</dd>
      <dt>OIDC subject</dt><dd>${escapeHtml(session.claims.sub)}</dd>
    </dl>
    ${session.claims.email_verified
      ? "<p>Your email address is verified.</p>"
        : `<p>Your email is not verified yet. In local development, open <a href="${escapeHtml(mailpitUrl)}">Mailpit</a>, open the verification email, then sign in again to get an updated email_verified claim.</p>`}
    <form method="post" action="/logout"><button type="submit">Sign out</button></form>
  `));
}

async function logout(request, response) {
  const origin = request.headers.origin;
  const expectedOrigin = new URL(redirectUri).origin;
  if (origin !== expectedOrigin) {
      response.writeHead(403, { "Content-Type": "text/plain; charset=utf-8" });
      response.end("The sign-out request origin is invalid.");
      return;
  }
  const sessionId = cookieValue(request, "af_demo_session");
  const session = sessionId ? sessions.get(sessionId) : undefined;
  if (session) {
    await postAuthForge("/v1/logout", {
      refresh_token: session.tokens.refresh_token,
    });
    sessions.delete(sessionId);
  }
  response.writeHead(303, {
    Location: "/",
    "Set-Cookie": cookie("af_demo_session", "", 0),
    "Cache-Control": "no-store",
  });
  response.end();
}

async function postAuthForge(path, body) {
  const result = await fetch(`${authforgeOrigin}${path}`, {
    method: "POST",
    redirect: "error",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  const payload = await result.json();
  if (!result.ok) {
    throw new Error(
      `AuthForge request failed (${result.status}): ${payload.error ?? "request_failed"}`,
    );
  }
  return payload;
}

function findSession(request) {
  const sessionId = cookieValue(request, "af_demo_session");
  return sessionId ? sessions.get(sessionId) : undefined;
}

function cookie(name, value, maxAge) {
  const secure = production ? "; Secure" : "";
  return `${name}=${encodeURIComponent(value)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${maxAge}${secure}`;
}

function cookieValue(request, name) {
  for (const part of (request.headers.cookie ?? "").split(";")) {
    const [key, ...value] = part.trim().split("=");
    if (key === name) {
      try {
        return decodeURIComponent(value.join("="));
      } catch {
        return "";
      }
    }
  }
  return "";
}

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (character) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  })[character]);
}

function page(title, content) {
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(title)} · AuthForge example</title>
  <style>
    body { font: 16px/1.5 system-ui, sans-serif; margin: 3rem auto; max-width: 42rem; padding: 0 1rem; color: #172033; }
    .button, button { display: inline-block; padding: .7rem 1rem; border: 0; border-radius: .5rem; background: #3949ab; color: white; font: inherit; text-decoration: none; cursor: pointer; }
    dt { font-weight: 700; margin-top: 1rem; } dd { margin-left: 0; overflow-wrap: anywhere; }
  </style>
</head>
<body><main><h1>${escapeHtml(title)}</h1>${content}</main></body>
</html>`;
}
