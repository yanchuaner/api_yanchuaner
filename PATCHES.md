# Yanchuaner patch inventory

Upstream baseline: New API `v1.0.0-rc.21` at `bde9b2f44887d34ec54799ae191d50f97914359e`.

## Local additions

- `deploy/`: isolated New API, LiteLLM, PostgreSQL, and Redis deployment for private testing.
- `docs/yanchuaner/`: architecture, security, and operations decisions.
- `scripts/generate-deploy-env.ps1`: local secret generation without committing credentials.
- `scripts/check-deploy-config.ps1`: deterministic deployment configuration checks.
- `YANCHUANER.md`: Yanchuaner-specific entry point while retaining all upstream project documentation and attribution.

## Core patches

- `oauth/*.go`: remove authorization-code fragments from all OAuth adapters; also remove token response bodies and user PII from generic OAuth debug logs. OAuth behavior and field mapping remain unchanged.

The first internal milestone otherwise leaves New API controller, billing, relay, model, and frontend code unchanged. Product-specific integration uses supported configuration and OAuth extension points.

## Upgrade rule

Every upstream update must record the previous and new commit, migration impact, billing regression results, and rollback image. Never merge an unpinned moving branch directly into the deployment branch.
