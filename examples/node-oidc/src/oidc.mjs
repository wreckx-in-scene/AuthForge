import {
  createHash,
  createPublicKey,
  randomBytes,
  verify,
} from "node:crypto";

const ed25519SpkiPrefix = Buffer.from("302a300506032b6570032100", "hex");

export function createOidcClient({
  issuer,
  clientId,
  redirectUri,
  fetchImpl = globalThis.fetch,
  now = () => Date.now(),
}) {
  if (typeof issuer !== "string" || !issuer.trim()) {
    throw new Error("AUTHFORGE_ISSUER is required");
  }
  if (typeof clientId !== "string" || !clientId.trim()) {
    throw new Error("AUTHFORGE_CLIENT_ID is required");
  }
  if (typeof fetchImpl !== "function") {
    throw new Error("A Fetch API implementation is required");
  }

  const issuerUrl = parseHttpUrl(issuer.replace(/\/+$/, ""), "issuer");
  requireSecureOrLocal(issuerUrl, "issuer");
  const callbackUrl = parseHttpUrl(redirectUri, "redirect URI");
  requireSecureOrLocal(callbackUrl, "redirect URI");
  if (callbackUrl.search || callbackUrl.hash) {
    throw new Error("The redirect URI must not contain a query or fragment");
  }
  const normalizedIssuer = issuerUrl.toString().replace(/\/+$/, "");

  async function getDiscovery() {
    const metadata = await fetchJson(
      new URL("/.well-known/openid-configuration", issuerUrl),
      fetchImpl,
    );
    if (metadata.issuer !== normalizedIssuer) {
      throw new Error("OIDC discovery issuer did not match AUTHFORGE_ISSUER");
    }

    for (const name of ["authorization_endpoint", "token_endpoint", "jwks_uri"]) {
      const endpoint = parseHttpUrl(metadata[name], `OIDC ${name}`);
      if (endpoint.origin !== issuerUrl.origin || endpoint.protocol !== issuerUrl.protocol) {
        throw new Error(`OIDC ${name} must use the AuthForge issuer origin`);
      }
    }
    if (
      !Array.isArray(metadata.response_types_supported) ||
      !metadata.response_types_supported.includes("code") ||
      !Array.isArray(metadata.code_challenge_methods_supported) ||
      !metadata.code_challenge_methods_supported.includes("S256") ||
      !Array.isArray(metadata.id_token_signing_alg_values_supported) ||
      !metadata.id_token_signing_alg_values_supported.includes("EdDSA")
    ) {
      throw new Error("AuthForge discovery does not advertise the required OIDC features");
    }
    return metadata;
  }

  return {
    async createAuthorizationRequest() {
      const metadata = await getDiscovery();
      const state = randomBytes(32).toString("base64url");
      const nonce = randomBytes(32).toString("base64url");
      const codeVerifier = randomBytes(32).toString("base64url");
      const codeChallenge = createHash("sha256")
        .update(codeVerifier, "ascii")
        .digest("base64url");
      const authorizationUrl = new URL(metadata.authorization_endpoint);
      authorizationUrl.search = new URLSearchParams({
        client_id: clientId,
        redirect_uri: redirectUri,
        response_type: "code",
        scope: "openid email",
        state,
        nonce,
        code_challenge: codeChallenge,
        code_challenge_method: "S256",
      }).toString();

      return {
        authorizationUrl: authorizationUrl.toString(),
        state,
        nonce,
        codeVerifier,
      };
    },

    async exchangeAuthorizationCode({ callbackUrl: returnedUrl, transaction }) {
      if (!transaction || typeof transaction !== "object") {
        throw new Error("The authorization transaction is missing");
      }
      const callback = parseHttpUrl(returnedUrl, "authorization callback");
      if (
        callback.origin !== callbackUrl.origin ||
        callback.pathname !== callbackUrl.pathname ||
        callback.hash ||
        callback.searchParams.getAll("state").length !== 1 ||
        callback.searchParams.get("state") !== transaction.state ||
        callback.searchParams.getAll("code").length !== 1
      ) {
        throw new Error("Authorization callback state or redirect URI did not match");
      }
      const code = callback.searchParams.get("code");
      if (!code || !transaction.codeVerifier || !transaction.nonce) {
        throw new Error("Authorization callback or PKCE transaction is incomplete");
      }

      const metadata = await getDiscovery();
      const tokenResponse = await fetchImpl(metadata.token_endpoint, {
        method: "POST",
        redirect: "error",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/x-www-form-urlencoded",
        },
        body: new URLSearchParams({
          grant_type: "authorization_code",
          client_id: clientId,
          code,
          redirect_uri: redirectUri,
          code_verifier: transaction.codeVerifier,
        }),
      });
      const tokens = await readJsonResponse(tokenResponse);
      if (!tokenResponse.ok) {
        throw new Error(
          `AuthForge token exchange failed (${tokenResponse.status}): ${safeErrorCode(tokens)}`,
        );
      }
      if (
        typeof tokens.access_token !== "string" ||
        typeof tokens.refresh_token !== "string" ||
        typeof tokens.id_token !== "string" ||
        typeof tokens.expires_in !== "number" ||
        !Number.isFinite(tokens.expires_in) ||
        tokens.expires_in < 0
      ) {
        throw new Error("AuthForge returned an incomplete token response");
      }

      const jwks = await fetchJson(metadata.jwks_uri, fetchImpl);
      const claims = verifyIdToken(tokens.id_token, {
        metadata,
        jwks,
        clientId,
        nonce: transaction.nonce,
        now: now(),
      });
      return {
        tokens: {
          ...tokens,
          expires_at: Math.floor(now() / 1000) + tokens.expires_in,
        },
        claims,
      };
    },
  };
}

export function verifyIdToken(token, { metadata, jwks, clientId, nonce, now = Date.now() }) {
  if (typeof token !== "string") {
    throw new Error("ID token must be a string");
  }
  const segments = token.split(".");
  if (segments.length !== 3) {
    throw new Error("ID token is malformed");
  }
  const header = decodeJsonSegment(segments[0], "ID token header");
  const claims = decodeJsonSegment(segments[1], "ID token claims");
  if (header.alg !== "EdDSA" || typeof header.kid !== "string" || !header.kid) {
    throw new Error("ID token signing algorithm or key ID is invalid");
  }
  if (!Array.isArray(jwks?.keys)) {
    throw new Error("OIDC JWKS response is malformed");
  }
  const jwk = jwks.keys.find((key) => key?.kid === header.kid);
  if (
    !jwk ||
    jwk.kty !== "OKP" ||
    jwk.crv !== "Ed25519" ||
    (jwk.use && jwk.use !== "sig") ||
    (jwk.alg && jwk.alg !== "EdDSA")
  ) {
    throw new Error("ID token verification key was not found in the JWKS");
  }

  const publicBytes = decodeBase64Url(jwk.x, "JWKS Ed25519 public key");
  if (publicBytes.length !== 32) {
    throw new Error("JWKS Ed25519 public key has an invalid length");
  }
  const derKey = Buffer.concat([ed25519SpkiPrefix, publicBytes]);
  const publicKey = createPublicKey({
    key: derKey,
    format: "der",
    type: "spki",
  });
  const signature = decodeBase64Url(segments[2], "ID token signature");
  if (
    signature.length !== 64 ||
    !verify(null, Buffer.from(`${segments[0]}.${segments[1]}`, "ascii"), publicKey, signature)
  ) {
    throw new Error("ID token signature is invalid");
  }

  const currentTime = Math.floor(now / 1000);
  const audiences = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  if (
    claims.iss !== metadata.issuer ||
    !audiences.includes(clientId) ||
    (audiences.length > 1 && claims.azp !== clientId) ||
    typeof claims.sub !== "string" ||
    !claims.sub ||
    typeof claims.exp !== "number" ||
    claims.exp <= currentTime ||
    typeof claims.iat !== "number" ||
    claims.iat > currentTime + 60 ||
    (typeof claims.nbf === "number" && claims.nbf > currentTime) ||
    claims.nonce !== nonce ||
    typeof claims.email !== "string" ||
    typeof claims.email_verified !== "boolean"
  ) {
    throw new Error("ID token issuer, audience, time, nonce, or identity claims are invalid");
  }
  return claims;
}

async function fetchJson(url, fetchImpl) {
  const response = await fetchImpl(url, {
    headers: { Accept: "application/json" },
    redirect: "error",
  });
  const payload = await readJsonResponse(response, url);
  if (!response.ok) {
    throw new Error(`${url} request failed (${response.status})`);
  }
  return payload;
}

async function readJsonResponse(response, url = "AuthForge") {
  let payload;
  try {
    payload = await response.json();
  } catch {
    throw new Error(`${url} returned invalid JSON`);
  }
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error(`${url} returned an invalid response`);
  }
  return payload;
}

function decodeJsonSegment(segment, name) {
  try {
    const value = JSON.parse(decodeBase64Url(segment, name).toString("utf8"));
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      throw new Error();
    }
    return value;
  } catch {
    throw new Error(`${name} is malformed`);
  }
}

function decodeBase64Url(value, name) {
  if (typeof value !== "string" || !/^[A-Za-z0-9_-]+$/.test(value)) {
    throw new Error(`${name} is malformed`);
  }
  return Buffer.from(value, "base64url");
}

function parseHttpUrl(value, name) {
  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`${name} must be an absolute URL`);
  }
  if (
    !["http:", "https:"].includes(parsed.protocol) ||
    parsed.username ||
    parsed.password
  ) {
    throw new Error(`${name} must be an HTTP(S) URL without credentials`);
  }
  return parsed;
}

function requireSecureOrLocal(url, name) {
  const localHost = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
  if (url.protocol !== "https:" && !(url.protocol === "http:" && localHost)) {
    throw new Error(`${name} must use HTTPS except on localhost`);
  }
}

function safeErrorCode(payload) {
  return typeof payload.error === "string" ? payload.error : "invalid_response";
}
