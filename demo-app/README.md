# juno-pay demo app

LocalStorage-only demo checkout UI for `juno-pay-server`.

## Env

Set these in your shell (or `.env.local`, not committed):

- `JUNO_PAY_BASE_URL` (example: `http://127.0.0.1:8080`)
- `JUNO_PAY_MERCHANT_API_KEY` (Bearer token for `POST /v1/invoices`)

## Run

```bash
npm install
npm run dev
```

