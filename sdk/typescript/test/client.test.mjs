import assert from "node:assert/strict";
import test from "node:test";

import { AuthForgeClient, AuthForgeError } from "../dist/index.js";

function jsonResponse(body, init = {}) {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  return new Response(JSON.stringify(body), { ...init, headers });
}

const tokens = {
  access_token: "access",
  refresh_token: "refresh",
  expires_at: 1_800_000_000,
  user_id: "user-1",
  session_id: "session-1",
  email_verified: false,
};

test("signup uses the environment key and parses the token response", async () => {
  let request;
  const client = new AuthForgeClient({
    apiKey: "af_test_public",
    baseUrl: "https://auth.example/",
    fetch: async (url, init) => {
      request = { url, init };
      return jsonResponse(tokens, { status: 201 });
    },
  });

  assert.deepEqual(
    await client.signUp({ email: "ada@example.com", password: "passphrase" }),
    tokens,
  );
  assert.equal(request.url, "https://auth.example/v1/signup");
  assert.equal(request.init.headers["X-Api-Key"], "af_test_public");
  assert.equal(request.init.headers["Content-Type"], "application/json");
  assert.deepEqual(JSON.parse(request.init.body), {
    email: "ada@example.com",
    password: "passphrase",
  });
});

test("Google sign-in requests a nonce and exchanges the ID token", async () => {
  const requests = [];
  const client = new AuthForgeClient({
    apiKey: "af_test_public",
    fetch: async (url, init) => {
      requests.push({ url, init });
      return jsonResponse(
        url.endsWith("/nonce") ? { nonce: "nonce-1" } : tokens,
      );
    },
  });

  assert.equal(await client.requestGoogleNonce(), "nonce-1");
  assert.deepEqual(
    await client.signInWithGoogle("id-token", "nonce-1"),
    tokens,
  );
  assert.deepEqual(
    JSON.parse(requests[1].init.body),
    { id_token: "id-token", nonce: "nonce-1" },
  );
});

test("refresh and logout omit the application API key", async () => {
  const requests = [];
  const client = new AuthForgeClient({
    apiKey: "af_test_public",
    fetch: async (url, init) => {
      requests.push({ url, init });
      return jsonResponse(
        url.endsWith("/refresh") ? tokens : { session_id: "session-1" },
      );
    },
  });

  await client.refresh("refresh");
  await client.logOut({ refreshToken: "refresh" });
  assert.equal("X-Api-Key" in requests[0].init.headers, false);
  assert.equal("X-Api-Key" in requests[1].init.headers, false);
});

test("authenticated requests and email verification use their documented auth", async () => {
  const requests = [];
  const client = new AuthForgeClient({
    apiKey: "af_test_public",
    fetch: async (url, init) => {
      requests.push({ url, init });
      if (url.endsWith("/me")) {
        return jsonResponse({
          user_id: "user-1",
          environment_id: "env-1",
          project_id: "project-1",
          session_id: "session-1",
          email_verified: true,
        });
      }
      if (url.endsWith("/request")) return jsonResponse({ status: "ok" });
      if (url.endsWith("/logout")) {
        return jsonResponse({ session_id: "session-1" });
      }
      return jsonResponse({ verified: true });
    },
  });

  assert.equal((await client.getCurrentUser("access")).email_verified, true);
  await client.requestEmailVerification("ada@example.com");
  assert.equal(await client.verifyEmail("env-1", "verification-token"), true);
  await client.logOut({ accessToken: "access" });
  assert.equal(requests[0].init.headers.Authorization, "Bearer access");
  assert.equal(requests[1].init.headers["X-Api-Key"], "af_test_public");
  assert.equal("X-Api-Key" in requests[2].init.headers, false);
  assert.equal(requests[3].init.headers.Authorization, "Bearer access");
  assert.equal("X-Api-Key" in requests[3].init.headers, false);
});

test("API errors preserve the server code and retry-after duration", async () => {
  const client = new AuthForgeClient({
    apiKey: "af_test_public",
    fetch: async () =>
      jsonResponse(
        { error: "rate_limited" },
        { status: 429, headers: { "Retry-After": "30" } },
      ),
  });

  await assert.rejects(client.logIn({ email: "a@b.c", password: "secret" }), {
    name: "AuthForgeError",
    status: 429,
    code: "rate_limited",
    retryAfter: 30,
  });
});

test("malformed success responses are rejected", async () => {
  const client = new AuthForgeClient({
    apiKey: "af_test_public",
    fetch: async () => jsonResponse({ access_token: "only-one-field" }),
  });

  await assert.rejects(
    client.logIn({ email: "a@b.c", password: "secret" }),
    (error) =>
      error instanceof AuthForgeError && error.code === "invalid_response",
  );
});

test("logout requires a refresh or access token", async () => {
  const client = new AuthForgeClient({ apiKey: "af_test_public" });
  await assert.rejects(client.logOut({}), /token is required/);
});
