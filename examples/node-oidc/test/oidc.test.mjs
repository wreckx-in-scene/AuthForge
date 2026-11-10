import assert from "node:assert/strict";
import {
  createHash,
  generateKeyPairSync,
  sign,
} from "node:crypto";
import test from "node:test";

import {
  createOidcClient,
  verifyIdToken,
} from "../src/oidc.mjs";

const issuer = "https://auth.example.test";
const clientId = "client-123";
const nonce = "nonce-456";
const metadata = {
  issuer,
  authorization_endpoint: `${issuer}/authorize`,
  token_endpoint: `${issuer}/v1/token`,
  jwks_uri: `${issuer}/.well-known/jwks.json`,
  response_types_supported: ["code"],
  code_challenge_methods_supported: ["S256"],
  id_token_signing_alg_values_supported: ["EdDSA"],
};
const { privateKey, publicKey } = generateKeyPairSync("ed25519");
const publicJwk = {
  ...publicKey.export({ format: "jwk" }),
  use: "sig",
  alg: "EdDSA",
  kid: "key-1",
};

function base64url(value) {
  return Buffer.from(JSON.stringify(value)).toString("base64url");
}

function issueToken(claimOverrides = {}, key = privateKey) {
  const encodedHeader = base64url({ alg: "EdDSA", kid: "key-1" });
  const encodedClaims = base64url({
    iss: issuer,
    aud: clientId,
    sub: "user-1",
    email: "ada@example.test",
    email_verified: false,
    nonce,
    iat: 1_800_000_000,
    exp: 1_800_000_300,
    ...claimOverrides,
  });
  const signingInput = `${encodedHeader}.${encodedClaims}`;
  const signature = sign(null, Buffer.from(signingInput), key).toString("base64url");
  return `${signingInput}.${signature}`;
}

test("validates EdDSA signature and required OIDC claims", () => {
  const claims = verifyIdToken(issueToken(), {
    metadata,
    jwks: { keys: [publicJwk] },
    clientId,
    nonce,
    now: 1_800_000_100_000,
  });
  assert.equal(claims.sub, "user-1");
  assert.equal(claims.email_verified, false);
});

test("rejects wrong issuer, audience, nonce, expired tokens, and signatures", () => {
  const options = {
    metadata,
    jwks: { keys: [publicJwk] },
    clientId,
    nonce,
    now: 1_800_000_100_000,
  };
  for (const claims of [
    { iss: "https://attacker.example" },
    { aud: "another-client" },
    { nonce: "another-nonce" },
    { exp: 1_800_000_000 },
  ]) {
    assert.throws(() => verifyIdToken(issueToken(claims), options));
  }
  const wrongKey = generateKeyPairSync("ed25519").privateKey;
  assert.throws(() => verifyIdToken(issueToken({}, wrongKey), options), /signature/);
});

test("authorization and callback complete with discovery, S256, state, and nonce", async () => {
  const requests = [];
  const client = createOidcClient({
    issuer,
    clientId,
    redirectUri: "https://app.example.test/auth/callback",
    now: () => 1_800_000_100_000,
    fetchImpl: async (url, options = {}) => {
      requests.push({ url: String(url), options });
      if (String(url).endsWith("/openid-configuration")) {
        return Response.json(metadata);
      }
      if (String(url).endsWith("/v1/token")) {
        const form = new URLSearchParams(options.body);
        assert.equal(form.get("grant_type"), "authorization_code");
        assert.equal(form.get("redirect_uri"), "https://app.example.test/auth/callback");
        assert.equal(form.get("code_verifier"), transaction.codeVerifier);
        return Response.json({
          access_token: "access",
          refresh_token: "refresh",
          id_token: issueToken({ nonce: transaction.nonce }),
          expires_in: 600,
        });
      }
      if (String(url).endsWith("/jwks.json")) {
        return Response.json({ keys: [publicJwk] });
      }
      throw new Error(`Unexpected request: ${url}`);
    },
  });

  const transaction = await client.createAuthorizationRequest();
  const authorizationUrl = new URL(transaction.authorizationUrl);
  const challenge = authorizationUrl.searchParams.get("code_challenge");
  const expectedChallenge = createHash("sha256")
    .update(transaction.codeVerifier, "ascii")
    .digest("base64url");
  assert.equal(challenge, expectedChallenge);
  assert.equal(authorizationUrl.searchParams.get("state"), transaction.state);
  assert.equal(authorizationUrl.searchParams.get("nonce"), transaction.nonce);
  assert.equal(authorizationUrl.searchParams.get("scope"), "openid email");

  const result = await client.exchangeAuthorizationCode({
    callbackUrl: `https://app.example.test/auth/callback?code=auth-code&state=${transaction.state}`,
    transaction,
  });
  assert.equal(result.tokens.access_token, "access");
  assert.equal(result.tokens.expires_at, 1_800_000_700);
  assert.equal(result.claims.sub, "user-1");
  assert.equal(requests.filter(({ url }) => url.endsWith("/jwks.json")).length, 1);
});

test("rejects callback state tampering and unsafe discovery endpoints", async () => {
  const client = createOidcClient({
    issuer,
    clientId,
    redirectUri: "https://app.example.test/auth/callback",
    fetchImpl: async () => Response.json(metadata),
  });
  const transaction = await client.createAuthorizationRequest();
  await assert.rejects(
    client.exchangeAuthorizationCode({
      callbackUrl: "https://app.example.test/auth/callback?code=code&state=wrong",
      transaction,
    }),
    /state or redirect URI/,
  );

  const unsafeClient = createOidcClient({
    issuer,
    clientId,
    redirectUri: "https://app.example.test/auth/callback",
    fetchImpl: async () =>
      Response.json({
        ...metadata,
        token_endpoint: "https://attacker.example/token",
      }),
  });
  await assert.rejects(unsafeClient.createAuthorizationRequest(), /issuer origin/);
});
