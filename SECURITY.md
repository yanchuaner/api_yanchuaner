# Security Policy

## Reporting

Do not open a public issue for credential exposure, authentication bypass, quota manipulation, billing errors, SSRF, cross-user data access, or BYOK leakage. Send a minimal encrypted or redacted report to `yanchuan_alumni@163.com` with subject `[燕中 API 安全报告]`.

Do not include live API keys, cookies, database dumps, prompts, member records, or production URLs that reveal private infrastructure. Provide a request ID, affected version, impact, and safe reproduction steps when possible.

## Supported Scope

Only the current 2026 summer preview branch is evaluated. The preview does not offer a commercial SLA, public recharge, refunds, invoices, or production BYOK.

## Security Invariants

- The main site is the only public identity source; password registration and login stay disabled.
- New Yanchuaner virtual keys are shown once and stored only as hashes.
- Public-benefit quota changes are append-only ledger entries with idempotency keys and audit context.
- Provider keys, OAuth client secrets, encryption keys, user data, risk rules, databases, and backups never enter Git.
- PostgreSQL, Redis, LiteLLM administration, profiling, and metrics are not exposed to the public network.
- Prompt and response bodies, Authorization, Cookie, BYOK plaintext, and supplier headers are not written to normal logs.

## Incident Order

Pause the affected channel, revoke downstream keys, rotate upstream credentials, freeze affected users, preserve redacted audit evidence, reconcile supplier charges, and restore the smallest approved model set.
