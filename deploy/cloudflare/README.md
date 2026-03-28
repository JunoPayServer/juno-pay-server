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

The DNS records are live and proxied through Cloudflare:

- `junopayserver.com A 18.206.49.27` with `proxied=true`
- `www.junopayserver.com A 18.206.49.27` with `proxied=true`
- `staging.junopayserver.com A 159.203.150.96` with `proxied=true`

As of `2026-03-28`, public delegation is already on the Cloudflare nameservers:

- `dig NS junopayserver.com +short` -> `chase.ns.cloudflare.com.`
- `dig NS junopayserver.com +short` -> `georgia.ns.cloudflare.com.`

Current public resolution still keeps production on AWS while staging points at DO:

- `dig junopayserver.com A +short` -> `18.206.49.27`
- `dig www.junopayserver.com A +short` -> `18.206.49.27`
- `dig staging.junopayserver.com A +short` -> `159.203.150.96`

## Current Access state

Access is enabled and live:

- `staging.junopayserver.com/*` is protected by Cloudflare Access
- `junopayserver.com/admin/*` and `junopayserver.com/v1/admin/*` are protected by Cloudflare Access
- `www.junopayserver.com/admin/*` and `www.junopayserver.com/v1/admin/*` are protected by Cloudflare Access
- public health, status, invoice, and checkout endpoints remain public
- the `junopayserver-admin-smoke` service token reaches staging and production admin paths correctly

## Current Load Balancing blocker

Load Balancing remains the hard blocker for production cutover.

Current behavior as of `2026-03-28`:

- the Cloudflare plugin now works for DNS and Access operations
- the Cloudflare plugin still fails on Load Balancing monitor creation with `10000: Authentication error`
- the Cloudflare plugin fails on load balancer record creation with `1002: load balancing not enabled for zone: validation failed`
- the Cloudflare plugin also lacks permission for the zone SSL settings endpoint, so `strict` was set via a temporary global-key API fallback
- the direct Cloudflare API with the temporary global key can read load balancer resources, but create calls still fail server-side

Exact direct API failures:

- monitor create:

  ```json
  {
    "result": null,
    "success": false,
    "errors": [
      {
        "code": 1002,
        "message": "interval is not in range [0, 0]: validation failed"
      }
    ],
    "messages": []
  }
  ```

- pool create:

  ```json
  {
    "result": null,
    "success": false,
    "errors": [
      {
        "code": 1002,
        "message": "Internal error creating or modifying pool. Access Failed. Please reach out to Support.: validation failed"
      }
    ],
    "messages": []
  }
  ```

- load balancer record create:

  ```json
  {
    "result": null,
    "success": false,
    "errors": [
      {
        "code": 1002,
        "message": "load balancing not enabled for zone: validation failed"
      }
    ],
    "messages": []
  }
  ```

Because monitor, pool, and load balancer record creation all fail, Cloudflare support must enable or repair Load Balancing on this zone before production cutover.

## Target LB configuration once unblocked

Create:

- one HTTPS health monitor for `GET /v1/health` with `Host: junopayserver.com`
- pool `aws-primary` -> `18.206.49.27`
- pool `do-secondary` -> `159.203.150.96`
- load balancer record for `junopayserver.com`
- load balancer record for `www.junopayserver.com`

Run pre-cutover with AWS as the active/default pool and DO as the healthy standby pool. Do not send production traffic to DO until the final maintenance window.

## Support escalation payload

Open a Cloudflare support case with:

- zone ID `6a7b914cfaab0d683a7a459dd9990816`
- account ID `03bd68eb1a12139579231d41799ca768`
- the exact plugin and direct API errors above
- the target LB configuration above
- the note that Access and proxied DNS already work for this zone, while LB create calls fail for both the plugin and direct API paths

Keep the support case in front of any production cutover work. The selected migration path is to wait for LB instead of switching production directly.
