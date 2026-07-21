# Yanchuaner patch inventory

Upstream baseline: New API `v1.0.0-rc.21` at `bde9b2f44887d34ec54799ae191d50f97914359e`.

## Local additions

- `deploy/`: isolated New API, LiteLLM, PostgreSQL, and Redis deployment for private testing.
- `docs/yanchuaner/`: architecture, security, and operations decisions.
- `scripts/generate-deploy-env.ps1`: local secret generation without committing credentials.
- `Dockerfile` / `.dockerignore`: exclude local toolchains and Go caches while including all required license and notice files in the release image.
- `scripts/check-deploy-config.ps1`: deterministic deployment configuration checks.
- `YANCHUANER.md`: Yanchuaner-specific entry point while retaining all upstream project documentation and attribution.
- `model/yanchuaner_virtual_key.go`: one-time virtual-key generation, hash lookup, and hash replay rejection.
- `model/yanchuaner_quota_ledger.go`: append-only public-benefit quota ledger with idempotent atomic balance projection.
- `controller/yanchuaner_quota_ledger.go`: self-service ledger query and audited administrator adjustment adapter.
- `SECURITY.md`, `CONTRIBUTING.md`, `THIRD_PARTY_NOTICES.md`: repository governance and disclosure boundaries.

## Core patches

- `oauth/*.go`: remove authorization-code fragments from all OAuth adapters; also remove token response bodies and user PII from generic OAuth debug logs. OAuth behavior and field mapping remain unchanged.
- `model/{main,user,token}.go`, `controller/{token,user}.go`, `middleware/auth.go`, `service/{funding_source,billing_session}.go`, `router/api-router.go`: thin compatibility adapters for hashed keys and the Yanchuaner ledger. These remain modified upstream files, not original Yanchuaner source.
- `web/default/src/features/keys/**`: show new virtual keys once and disable later plaintext retrieval; preserve upstream copyright headers.
- `web/default/src/i18n/{config,languages}.ts`: expose only Simplified Chinese and English at runtime while retaining upstream locale files for upgrade provenance.

Legacy top-up, check-in, subscription, asynchronous task, redemption, and BYOK paths are not migrated to the Yanchuaner ledger. They remain disabled for the preview and must not be described as completed autonomous control-plane features.

## Upgrade rule

Every upstream update must record the previous and new commit, migration impact, billing regression results, and rollback image. Never merge an unpinned moving branch directly into the deployment branch.
