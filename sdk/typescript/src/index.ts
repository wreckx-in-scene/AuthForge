export interface AuthForgeClientOptions {
  apiKey: string;
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
}

export interface Credentials {
  email: string;
  password: string;
}

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  expires_at: number;
  user_id: string;
  session_id: string;
  email_verified: boolean;
}

export interface CurrentUser {
  user_id: string;
  environment_id: string;
  project_id: string;
  session_id: string;
  email_verified: boolean;
}

export interface LogoutResult {
  session_id: string;
}

export class AuthForgeError extends Error {
  readonly status: number;
  readonly code: string;
  readonly retryAfter?: number;

  constructor(status: number, code: string, retryAfter?: number) {
    super(`AuthForge API request failed: ${code}`);
    this.name = "AuthForgeError";
    this.status = status;
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

type RequestOptions = {
  method?: "GET" | "POST";
  body?: Record<string, string>;
  accessToken?: string;
  includeApiKey?: boolean;
};

export class AuthForgeClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly fetchImpl: typeof globalThis.fetch;

  constructor(options: AuthForgeClientOptions) {
    if (!options.apiKey.trim()) {
      throw new Error("AuthForge API key is required");
    }

    const baseUrl = options.baseUrl ?? "http://localhost:8080";
    if (!baseUrl.trim()) {
      throw new Error("AuthForge base URL is required");
    }

    const fetchImpl = options.fetch ?? globalThis.fetch;
    if (typeof fetchImpl !== "function") {
      throw new Error("A Fetch API implementation is required");
    }

    this.apiKey = options.apiKey.trim();
    this.baseUrl = baseUrl.replace(/\/+$/, "");
    this.fetchImpl = fetchImpl;
  }

  signUp(credentials: Credentials): Promise<AuthTokens> {
    return this.requestTokens("/v1/signup", {
      body: { email: credentials.email, password: credentials.password },
    });
  }

  logIn(credentials: Credentials): Promise<AuthTokens> {
    return this.requestTokens("/v1/login", {
      body: { email: credentials.email, password: credentials.password },
    });
  }

  async requestGoogleNonce(): Promise<string> {
    const response = await this.request("/v1/oauth/google/nonce");
    return stringField(response, "nonce");
  }

  signInWithGoogle(idToken: string, nonce: string): Promise<AuthTokens> {
    return this.requestTokens("/v1/oauth/google", {
      body: { id_token: idToken, nonce },
    });
  }

  async requestEmailVerification(email: string): Promise<void> {
    const response = await this.request("/v1/email/verification/request", {
      body: { email },
    });
    if (stringField(response, "status") !== "ok") {
      throw new AuthForgeError(200, "invalid_response");
    }
  }

  async verifyEmail(environmentId: string, token: string): Promise<boolean> {
    const response = await this.request("/v1/email/verification/confirm", {
      body: { environment_id: environmentId, token },
      includeApiKey: false,
    });
    return booleanField(response, "verified");
  }

  refresh(refreshToken: string): Promise<AuthTokens> {
    return this.requestTokens("/v1/token/refresh", {
      body: { refresh_token: refreshToken },
      includeApiKey: false,
    });
  }

  async logOut(options: {
    refreshToken?: string;
    accessToken?: string;
  }): Promise<LogoutResult> {
    if (!options.refreshToken && !options.accessToken) {
      throw new Error("A refresh token or access token is required to log out");
    }
    const response = await this.request("/v1/logout", {
      body: options.refreshToken ? { refresh_token: options.refreshToken } : {},
      accessToken: options.refreshToken ? undefined : options.accessToken,
      includeApiKey: false,
    });
    return { session_id: stringField(response, "session_id") };
  }

  async getCurrentUser(accessToken: string): Promise<CurrentUser> {
    const response = await this.request("/v1/me", {
      method: "GET",
      accessToken,
      includeApiKey: false,
    });
    return {
      user_id: stringField(response, "user_id"),
      environment_id: stringField(response, "environment_id"),
      project_id: stringField(response, "project_id"),
      session_id: stringField(response, "session_id"),
      email_verified: booleanField(response, "email_verified"),
    };
  }

  private async requestTokens(
    path: string,
    options: RequestOptions,
  ): Promise<AuthTokens> {
    const response = await this.request(path, options);
    return {
      access_token: stringField(response, "access_token"),
      refresh_token: stringField(response, "refresh_token"),
      expires_at: numberField(response, "expires_at"),
      user_id: stringField(response, "user_id"),
      session_id: stringField(response, "session_id"),
      email_verified: booleanField(response, "email_verified"),
    };
  }

  private async request(
    path: string,
    options: RequestOptions = {},
  ): Promise<Record<string, unknown>> {
    const headers: Record<string, string> = {
      Accept: "application/json",
    };
    if (options.includeApiKey !== false) {
      headers["X-Api-Key"] = this.apiKey;
    }
    if (options.body) {
      headers["Content-Type"] = "application/json";
    }
    if (options.accessToken) {
      headers.Authorization = `Bearer ${options.accessToken}`;
    }

    const response = await this.fetchImpl(`${this.baseUrl}${path}`, {
      method: options.method ?? "POST",
      headers,
      body: options.body ? JSON.stringify(options.body) : undefined,
    });
    const payload = await readObject(response);
    if (!response.ok) {
      const code =
        typeof payload.error === "string" ? payload.error : "request_failed";
      const retryAfterHeader = response.headers.get("Retry-After");
      const retryAfter = retryAfterHeader?.trim()
        ? Number(retryAfterHeader)
        : undefined;
      throw new AuthForgeError(
        response.status,
        code,
        retryAfter !== undefined && Number.isFinite(retryAfter) && retryAfter >= 0
          ? retryAfter
          : undefined,
      );
    }
    return payload;
  }
}

async function readObject(
  response: Response,
): Promise<Record<string, unknown>> {
  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    throw new AuthForgeError(response.status, "invalid_response");
  }
  if (!isRecord(payload)) {
    throw new AuthForgeError(response.status, "invalid_response");
  }
  return payload;
}

function isRecord(payload: unknown): payload is Record<string, unknown> {
  return (
    typeof payload === "object" &&
    payload !== null &&
    !Array.isArray(payload)
  );
}

function stringField(
  payload: Record<string, unknown>,
  field: string,
): string {
  const value = payload[field];
  if (typeof value !== "string") {
    throw new AuthForgeError(200, "invalid_response");
  }
  return value;
}

function numberField(
  payload: Record<string, unknown>,
  field: string,
): number {
  const value = payload[field];
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new AuthForgeError(200, "invalid_response");
  }
  return value;
}

function booleanField(
  payload: Record<string, unknown>,
  field: string,
): boolean {
  const value = payload[field];
  if (typeof value !== "boolean") {
    throw new AuthForgeError(200, "invalid_response");
  }
  return value;
}
