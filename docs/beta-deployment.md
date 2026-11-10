# Small controlled beta deployment

This deployment runs the existing Compose services on **one Linux VM** behind
Caddy-managed HTTPS. It is suitable only for a small, manually coordinated
preview where users understand that there is no service-level guarantee. It is
not production-ready or high-availability. Developer console signup is gated
by a shared beta invite code configured on the gateway.

The Docker Compose files do not create a cloud account or guarantee a free
host. Choose a provider's currently available free compute tier or a machine
you control, check its limits and billing requirements, and set a spending
limit before exposing real user accounts. Free-tier availability varies by
region/provider. The VM needs a stable public IP, inbound TCP ports 80/443,
enough persistent disk for PostgreSQL, and a DNS name pointed at that IP.

## Before launch

- Use a dedicated VM and a DNS name you control. Do not use the example domain.
- Create `.env.beta` from `.env.beta.example`; never commit it.
- Set every secret to a fresh random value. Keep the signing seed backed up
  securely: replacing it invalidates issued access and ID tokens.
- Configure SMTP credentials and a sender domain authorized by your provider.
  The beta overlay forces STARTTLS.
- Restrict administrative SSH and database access. PostgreSQL and Mailpit are
  not published to the public host; only Caddy's ports 80/443 are exposed.
- Confirm the provider's data-retention and backup behavior before onboarding
  users. Make and test encrypted database backups before launch.
- To restrict developer console signup to invited testers, set CONSOLE_INVITE_CODE
  in `.env.beta`. Generate a long random code, require testers to enter it when
  creating a console account, and share it privately. This is a shared code,
  not an individual, expiring invitation.

On a Linux host with Docker Compose v2:

```bash
cp .env.beta.example .env.beta
chmod 600 .env.beta
openssl rand -hex 32
openssl rand -base64 32
```

Generate another `openssl rand -hex 32` value for `CONSOLE_INVITE_CODE`.
Paste distinct generated values into the password/secret fields. Use a
base64-encoded 32-byte value for `SIGNING_SEED`; use a different random secret
for `CONSOLE_TOKEN_SECRET`. Set a real SMTP address and sender. The Compose
configuration fails validation if `CONSOLE_INVITE_CODE` is unset or empty.
Then validate the merged configuration without printing secret values:

```bash
docker compose --env-file .env.beta \
  -f docker-compose.yml -f docker-compose.beta.yml config --quiet
```

Start the stack:

```bash
docker compose --env-file .env.beta \
  -f docker-compose.yml -f docker-compose.beta.yml up --build -d
```

Caddy will request and renew an HTTPS certificate once public DNS resolves to
the host and ports 80/443 are reachable. Verify the HTTPS discovery endpoint
and try a complete test signup, email verification, sign-in, and logout before
inviting anyone:

```bash
curl --fail https://your-authforge-domain/.well-known/openid-configuration
```

To inspect service health:

```bash
docker compose --env-file .env.beta \
  -f docker-compose.yml -f docker-compose.beta.yml ps
```

## Data and rollback

The default PostgreSQL named volume survives `docker compose down`, but it is
not a backup. Never run `docker compose down --volumes` on a beta installation.
Keep encrypted off-host backups and regularly test restoring one. Test schema
migrations against a copy of the database before applying them to users.

The migration container uses the PostgreSQL administrator credential. The
application services use the separate `authforge_app` role; the one-shot
`db-credentials` job sets its password from `.env.beta` after migrations.
This keeps normal service connections subject to the identity/session RLS
policies.

## Known limits

- One VM and one PostgreSQL instance are a single point of failure.
- Compose's rate limiter is process-local and resets on gateway restart.
- Internal gRPC is plaintext on the private Compose network.
- The application has no automated backup scheduler or restore workflow.
- The console uses one shared signup code; it does not support individual
  invitations, expiration, or per-invite usage limits.
- Console accounts have no team invitations or role management.
- Email verification does not block initial login; applications must check
  `email_verified` where their policy requires verified email.
- Free compute can sleep, disappear, or change its allowances; do not promise
  uptime or recovery to beta users.

Do not use this deployment for sensitive or regulated workloads. Before
expanding beyond a small controlled beta, add managed secrets/key rotation,
durable monitoring and alerting, shared rate limiting, disaster-recovery
drills, service-to-service authentication, and an independent security review.
