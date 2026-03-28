# Cloudflare migration state

This folder documents the Cloudflare side of the `junopayserver.com` migration.

## Current zone state

The zone already exists in the connected Cloudflare account:

- Zone: `junopayserver.com`
- Zone ID: `6a7b914cfaab0d683a7a459dd9990816`
- Account ID: `03bd68eb1a12139579231d41799ca768`
- Cloudflare nameservers:
  - `chase.ns.cloudflare.com`
  - `georgia.ns.cloudflare.com`
- SSL/TLS mode: `strict`

## Current DNS records in Cloudflare

The public hostnames are live and proxied through Cloudflare:

- `junopayserver.com` is served by a Cloudflare Load Balancer with `proxied=true`
- `www.junopayserver.com` is served by a Cloudflare Load Balancer with `proxied=true`
- `staging.junopayserver.com A 159.203.150.96` with `proxied=true`

As of `2026-03-28`, public delegation is already on the Cloudflare nameservers:

- `dig NS junopayserver.com +short` -> `chase.ns.cloudflare.com.`
- `dig NS junopayserver.com +short` -> `georgia.ns.cloudflare.com.`

Current public resolution returns Cloudflare anycast IPs for the production names, while staging still points at the DO origin through the proxy:

- `dig junopayserver.com A +short` -> Cloudflare anycast IPs
- `dig www.junopayserver.com A +short` -> Cloudflare anycast IPs
- `dig staging.junopayserver.com A +short` -> Cloudflare anycast IPs

## Current Access state

Access is enabled and live:

- `staging.junopayserver.com/*` is protected by Cloudflare Access
- `junopayserver.com/admin/*` and `junopayserver.com/v1/admin/*` are protected by Cloudflare Access
- `www.junopayserver.com/admin/*` and `www.junopayserver.com/v1/admin/*` are protected by Cloudflare Access
- public health, status, invoice, and checkout endpoints remain public
- the `junopayserver-admin-smoke` service token reaches staging and production admin paths correctly

## Current Load Balancing state

Load Balancing is enabled and live for production hostnames.

Current behavior as of `2026-03-28`:

- the Cloudflare plugin works for DNS and Access operations
- the Cloudflare plugin still fails on LB writes, so LB resources were created with the temporary global-key fallback
- the Cloudflare plugin also lacks permission for the zone SSL settings endpoint, so `strict` was set via a temporary global-key API fallback
- the global-key API path now succeeds for monitor, pool, and LB creation after the product was enabled

Current live LB resources:

- monitor `d01e47da9cae574c77cffeb5592d7d6c`
  - type `https`
  - `GET /v1/health`
  - `Host: junopayserver.com`
  - interval `60s`
- pool `aws_primary` (`4f638d5ea23116d07e1cf1461524a716`)
  - origin `18.206.49.27`
  - healthy
- pool `do_secondary` (`26af9c8e599b3185f6d5dc4a8a283d40`)
  - origin `159.203.150.96`
  - healthy
- load balancer `junopayserver.com` (`429e1bd9e2e202fb83fdc05250fce2ef`)
- load balancer `www.junopayserver.com` (`0de5f8521b63674960d049ca25e7c8d6`)

Current traffic posture:

- AWS is first in `default_pools`
- DO is second in `default_pools`
- `fallback_pool` is AWS
- production traffic stays on AWS until the final maintenance window
- DO is monitored as healthy standby only

## Remaining Cloudflare caveat

The remaining Cloudflare gap is connector-only:

- the Cloudflare plugin still cannot manage LB writes
- the Cloudflare plugin still cannot update the zone SSL setting

If future LB edits are needed before the plugin permissions are fixed, use the same temporary global-key fallback and document the change in this runbook.
