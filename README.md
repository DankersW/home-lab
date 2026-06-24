# Home Lab

Self hosting open source alternative to expensive subscriptions

## Networking & access

Public traffic enters through a single Cloudflare Tunnel (`cloudflared`) and is
forwarded to Traefik, which routes by hostname. The tunnel is **locally
managed**: every route lives in
[`containers/cloudflared/config.yml`](containers/cloudflared/config.yml) and is
applied on startup.

**Expose a new service:**

1. Add a router + service under `containers/traefik/dynamic/`.
2. Add a `- hostname: <name>.dankers.io` line to `containers/cloudflared/config.yml`.

DNS and auth need no per-service work — two one-time wildcards (below) cover
every subdomain.

### One-time setup

This reuses the existing tunnel, so it's done once (e.g. over SSH on the Pi).

**1. Tunnel credentials (on the host).** A locally-managed tunnel authenticates
with a credentials JSON rather than the token. Convert the token you already
have, then drop it:

```sh
cd ~/home-lab
python3 -c 'import base64, json; t = open("infra/secrets/cloudflared_token").read().strip(); d = json.loads(base64.b64decode(t + "=" * (-len(t) % 4))); json.dump({"AccountTag": d["a"], "TunnelID": d["t"], "TunnelSecret": d["s"]}, open("infra/secrets/cloudflared_credentials", "w"))'
rm infra/secrets/cloudflared_token
```

**2. Wildcard DNS** — Cloudflare dashboard → DNS → Records → add:

| Type  | Name | Target                                                  | Proxy   |
|-------|------|---------------------------------------------------------|---------|
| CNAME | `*`  | `7c3acfe6-f333-4ec3-bb6b-759c159c731d.cfargotunnel.com` | Proxied |

New subdomains then need no DNS changes.

**3. Wildcard Access / SSO** — Cloudflare dashboard → Zero Trust → Access →
Applications → Add an application → Self-hosted:

- Application domain: `*.dankers.io`
- Attach your SSO/identity policy (e.g. allow your email domain).
- Save. Cloudflare now authenticates every subdomain before it reaches the
  tunnel and injects `Cf-Access-Authenticated-User-Email`, which the apps trust.

> Switching to a credentials file makes Cloudflare mark the tunnel **"locally
> managed"**; its old dashboard Public Hostname entries become inert and can be
> deleted.

Then `make up` (or a push to `main`) brings the stack online with all routes
configured from `config.yml`.

## Provisioning the Pi

The server runs on a Raspberry Pi 4 (headless Ubuntu Server 24.04). To flash a
card that boots fully configured — static IP, key-only SSH, Docker installed —
see [`infra/pi/`](infra/pi/README.md):

```sh
make flash DEVICE=/dev/sdX
```

Then `ssh wouter@homelab.local` and `make up` to bring the stack online.
