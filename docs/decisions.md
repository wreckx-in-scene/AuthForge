# Design decisions

These are the choices Milestone 01 is built on. Each one names the alternative it turned down and the consequence that shows up in the code.

## 1. The environment is the isolation boundary

Context. The product has organizations, projects, and environments. A college project uses the same codebase in development and production.

Decision. Users, credentials, sessions, and API keys belong to an environment. The unique login is `(environment_id, normalized_email)`. The access-token audience is the environment id.

Alternative. Scope users to the project, which is what the first table sketch said. Development and production would then share accounts, so a test signup would be a production user.

Tradeoff. A person who wants one account in both environments signs up twice. That is the same model as two deployments of the application. Promoting data from development to production is an explicit later feature, not an accident of the key.

## 2. Developers and application users are different identities

Context. The person who creates the CampusConnect organization also has end users inside CampusConnect.

Decision. Console accounts live in `tenant.accounts`. Application users live in `identity.users`. Membership in an organization does not create an application user, and an application user is not a member.

Alternative. One user table with a tenant id. That collapses the two products into one authorization model and makes "a developer belongs to several organizations" the same fact as "an end user belongs to one app."

Tradeoff. Console login cannot be dogfooded through the application signup path yet. The console uses its own HMAC tokens. When the console itself is an AuthForge project, this second stack can go away.

## 3. Each service owns its tables

Context. Five processes need to agree on a user and a session.

Decision. `tenant`, `identity`, and `session` are separate schemas. A service writes only its schema. There is no foreign key from `session.sessions` to `identity.users`. Auth copies the environment id and the project id from the API-key resolution into both calls.

Alternative. One shared schema and foreign keys across every relationship. Cross-project references become a database error, which is attractive, and every service can also update every table.

Tradeoff. A bug in auth could store a session for a user id that identity does not have. The gateway prevents the client from choosing those ids. A cross-service foreign key would catch the bug and would also couple migrations and process releases. The integration test is what keeps the orchestrator honest.

Row-level security is an extra guard on identity and session, not the authorization layer. The repository still puts the environment id in the statement. RLS is forced for the non-superuser role `authforge_app`. The Compose database user that runs migrations is a superuser, and superusers bypass RLS, so the services do not connect as that user.

The tenant schema has no RLS. Its rule is organization membership, which is a different predicate.

## 4. Public HTTP, internal gRPC

Context. The backend wants typed service calls. Application developers already know HTTP and, later, OIDC.

Decision. The gateway is the only process with a published application port. It speaks JSON. The other services speak gRPC and are reachable only on the Compose network.

Alternative. Publish the gRPC services and give developers a proto file. That avoids a translation layer and asks every caller to adopt the internal contract, including clients that will eventually use OIDC.

Tradeoff. Every new field is added twice, once in the proto and once in JSON. The gateway is also the place that has to reject a body which tries to name an environment. That duplication is the product boundary.

Passwords travel in the gRPC request body on the private network. They are not placed in metadata, because metadata is what interceptors and proxies log by habit.

## 5. The gateway is the trust boundary for API keys

Context. Identity and session need an environment id, but they must not trust one supplied by the browser.

Decision. The gateway resolves `X-Api-Key` through the tenant service and passes the returned ids to auth. Internal services are not published on the host. In production they also need mTLS so a process that merely shares a network cannot call `CreateUser`. Compose does not terminate TLS in this milestone.

Alternative. Every service validates the raw API key. That keeps the gateway thin and makes every service a client of the tenant schema or the tenant RPC.

Tradeoff. A bug in the gateway can pass the wrong environment id. The key resolution returns the environment and the project together from one query, and the handler forwards that pair unchanged.

## 6. Ed25519 access tokens, opaque refresh tokens

Context. A protected request should be cheap. Logout and theft detection need a server-side record.

Decision. Auth signs access tokens with Ed25519. The private seed is an environment variable, not a column. The gateway verifies from a public key it fetches at startup. The header `kid` is `af-ed25519-1`. A second key can be accepted during rotation by adding another public key to that map. Refresh tokens are random, prefixed with `af_rt_`, and stored as a SHA-256 hash.

Alternative. A per-project HMAC secret in the project row, which is what the first schema did. It makes every project a signer and puts the signing material next to the API-key hashes. It also does not produce a JWKS document a third party can fetch.

Tradeoff. Until expiry, a stolen access token still works after logout. The default lifetime is ten minutes. `GET /v1/me` does not read the session table, so this behavior is visible and testable rather than hidden behind an accidental database lookup.

Console tokens are HMAC-SHA256 and only the console routes accept them. The verifiers pin the algorithm. An access token cannot be checked with the console secret, which is the usual algorithm-confusion bug.

## 7. Refresh rotation fails closed

Context. A refresh token that can be used forever is a second password. A refresh token that stays valid after rotation can be stolen quietly.

Decision. Rotation is one transaction. The presented hash is locked, marked used, and a child row is inserted. Presentation of a used token revokes the family. Presentation of a token that logout already revoked is an authentication failure, not a theft signal.

Alternative. A short grace period where the old token and the new token both work, so a client that missed the response can retry. That needs the plaintext of the new token to be recoverable, or the client will still be unable to continue.

Tradeoff. A lost HTTP response followed by a retry revokes the family and forces a fresh login. The user sees a logout. An attacker who races the real client gets the same outcome. The milestone prefers that over a replay window.

## 8. Signup does not pretend to be one transaction

Context. The user row and the session row live in different schemas and different processes.

Decision. Order is fixed: identity commit, session commit, sign, respond. Unique violation means the signup lost the race. A session error after a user commit returns an error and no tokens. A signing error revokes the new session before returning.

Alternative. An outbox in identity that creates the session asynchronously. The client would not receive tokens in the signup response, which is the whole point of this milestone. Two-phase commit across Postgres schemas was rejected because the services are supposed to be able to move to separate databases later.

Tradeoff. The system can contain a user with no session, or a session whose tokens were never delivered. It must not contain a response whose tokens do not match a committed session. Callers recover by logging in.

Email verification is not on this path. The access token says `email_verified: false`. A later project setting can require the claim. This milestone does not block login on mail delivery.

## 9. Password and key storage

Decision. Passwords use Argon2id with 64 MiB of memory, 3 iterations, parallelism 4, a 16-byte salt, and a 32-byte tag, stored as a PHC string. Verification reads the parameters from that string and refuses limits above a small ceiling, so a tampered row cannot request a huge hash. API keys and refresh tokens use SHA-256 because the secret is already high entropy and the lookup is by hash.

Email normalization is trim and lowercase. Dots and plus-tags are preserved. Two Gmail spellings that a human considers the same can be two users. The rule is simple enough to explain and to index.

API keys are shown once. The stored prefix is long enough to recognize a key in a dashboard and short enough to be useless for authentication. The prefix of a production environment is `af_live_`. Every other environment uses `af_test_`. Publishable and secret keys are both accepted on signup and login in this milestone. Management routes do not accept them at all.

## 10. One module, five images, one migration job

Decision. The repository is one Go module so the proto package, the gateway, and the services build together. Each service still has its own `main` package and its own image. A one-shot `migrate` container applies the three schemas as the database superuser before the services start. The services connect as `authforge_app`.

Alternative. A Go workspace with a module per service from day one. That matches independent deployment and makes local changes across a proto and a caller painful before there is a consumer outside this repo.

Tradeoff. A bad import can reach into another service's store package. The boundary is convention plus the fact that each image only runs one `main`. Generated protobuf code is committed so an image build does not need the protobuf toolchain.

Migrations are not run automatically by every replica. Two containers applying the same migration is a problem the one-shot job avoids. The cost is an extra Compose step that must finish before the services are healthy.

## 11. What this milestone refuses to build

- Kafka and a transactional outbox. Nothing asynchronous has to be delivered yet.
- A denylist of access-token ids. Short expiry is the revocation story for access tokens.
- mTLS, Redis rate limits, and a JWKS HTTP document. The gateway already treats the auth service as the source of the verification key, which is the same relationship a JWKS URL will have.
- Hosted login, dashboard, and SDKs. The curl flow against the gateway is the contract those clients will wrap.
