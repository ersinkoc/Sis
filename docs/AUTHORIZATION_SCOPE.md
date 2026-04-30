# Authorization Scope

Decision date: 2026-04-30

ASSUMPTION: The current v1 release scope targets tightly controlled home, lab, and
small-office deployments where the management surface is restricted to localhost, VPN,
trusted management networks, or a protected reverse proxy.

## Decision

Role-based admin permissions are explicitly deferred from the current v1 release scope.
Every authenticated local user is treated as a full administrator.

This matches the current implementation:

- `/api/v1/auth/setup` and `/api/v1/auth/login` establish local cookie sessions.
- All other `/api/v1/*` routes require an authenticated session.
- There is no per-route role check, resource ownership model, or read-only operator role.
- The WebUI and HTTP-backed CLI use the same authenticated management API.

## Accepted For V1

This is acceptable only for the documented small-site posture:

- Bind management HTTP to localhost or a trusted management network.
- Use TLS and `auth.secure_cookie` when management is exposed through a reverse proxy.
- Keep user count small and limited to trusted operators.
- Treat every account as able to change DNS policy, upstreams, clients, groups, config,
  cache, logs, store verification, and future administrative endpoints.

## Not Accepted For Broad Production

Broad production, managed-service, shared-tenant, or untrusted-network deployments need a
stronger authorization model before release claims are expanded.

Required future decisions:

1. Define roles such as owner/admin, operator, and read-only auditor.
2. Map every HTTP route and CLI command to required permissions.
3. Add audit entries that include actor and permission context for sensitive changes.
4. Decide whether external identity, OIDC, or reverse-proxy auth is required.
5. Add regression tests proving forbidden mutations fail for lower-privilege users.

## Current Risk

Compromise or misuse of any authenticated account is equivalent to full administrative
compromise of the Sis management plane. The supported mitigation for v1 is network
isolation, TLS, strong local passwords, short-lived sessions where appropriate, backups,
and audit/log review.
