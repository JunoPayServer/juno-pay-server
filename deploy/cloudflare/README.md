# Cloudflare migration state

This folder documents the Cloudflare side of the `junopayserver.com` migration.

## Current zone state

The zone already exists in the connected Cloudflare account:

- Zone: `junopayserver.com`
- Zone ID: `6a7b914cfaab0d683a7a459dd9990816`
- Cloudflare nameservers:
  - `chase.ns.cloudflare.com`
  - `georgia.ns.cloudflare.com`

## Current DNS records in Cloudflare

The DNS records were reconciled during implementation to match the migration plan:

- `junopayserver.com A 18.206.49.27` with `proxied=false`
- `www.junopayserver.com A 18.206.49.27` with `proxied=false`
- `staging.junopayserver.com A 159.203.150.96` with `proxied=false`

As of `2026-03-28`, public delegation is already on the Cloudflare nameservers:

- `dig NS junopayserver.com +short` -> `chase.ns.cloudflare.com.`
- `dig NS junopayserver.com +short` -> `georgia.ns.cloudflare.com.`

Current public resolution still keeps production on AWS while staging points at DO:

- `dig junopayserver.com A +short` -> `18.206.49.27`
- `dig www.junopayserver.com A +short` -> `18.206.49.27`
- `dig staging.junopayserver.com A +short` -> `159.203.150.96`

## Current plugin limitation

The Cloudflare plugin is currently blocked for follow-on work:

- earlier in implementation, DNS reads and writes succeeded
- later Access and Load Balancing requests failed with `10000: Authentication error`
- the latest verification on `2026-03-28` failed with `9109: Invalid access token`

Treat Cloudflare plugin work as unavailable until the app connection is repaired. The blocked areas include:

- Zone-level Access APIs
- Account-level Zero Trust Access APIs
- Load Balancing health checks
- Load Balancing pools
- Load Balancing load balancers

That means DNS staging is implemented, but Access and Load Balancing remain blocked until the Cloudflare plugin is reconnected or granted the missing scopes/products.

## Pending Cloudflare work once plugin auth is fixed

### Access

Create:

- a self-hosted Access application for `staging.junopayserver.com/*`
- a path-scoped Access application for `junopayserver.com/admin/*`
- a path-scoped Access application for `junopayserver.com/v1/admin/*`
- one Access service token for automation and smoke tests

Keep these paths public:

- `junopayserver.com/v1/health`
- `junopayserver.com/v1/status`
- public invoice and checkout endpoints under `/v1/public/*`

### Load Balancing

Create:

- one HTTPS health check for `/v1/health` with `Host: junopayserver.com`
- pool `aws-primary` -> `18.206.49.27`
- pool `do-secondary` -> `159.203.150.96`
- load balancer `junopayserver.com`
- load balancer `www.junopayserver.com`

Use AWS as the fallback/default pool until the final maintenance cutover.
