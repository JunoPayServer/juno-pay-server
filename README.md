# juno-pay-server

Self-hosted payment backend for the Juno Cash ecosystem.

- Canonical API schema: `api/openapi.yaml`
- Admin UI: `/admin` (password from env)
- Demo checkout UI: `demo-app/` (localStorage-only)
- Chain + scanning: requires `junocashd` + `juno-scan` (local IPC preferred)
- Receive-only: merchants maintain their own spending wallets (server stores UFVK only)
- Invoice expiry + policies: configured per-merchant in admin (stored in DB)
