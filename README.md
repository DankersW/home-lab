# Home Lab

Self hosting open source alternative to expensive subscriptions

```sh
ssh wouter@homelab.local
```

## Networking & access

Public traffic enters through a single Cloudflare Tunnel (`cloudflared`) and is
forwarded to Traefik, which routes by hostname. The tunnel is **locally
managed**: every route lives in
[`containers/cloudflared/config.yml`](containers/cloudflared/config.yml) and is
applied on startup.

## Local development

Run the app services on `127.0.0.1` without the edge (no Traefik, no Cloudflare
Tunnel). Cloudflare Access is absent locally, so dev disables auth on `receipts`
and `dozzle` and remaps the colliding `:8080` ports.

```sh
make bootstrap  # once: create data dirs + minio secrets
make up         # build + start (receipts, minio, dozzle)
make logs       # follow logs
make down       # stop
```

| Service | URL |
| --- | --- |
| receipts | http://localhost:8080 |
| minio (console) | http://localhost:9001 |
| dozzle | http://localhost:8082 |

The stack is defined in [`dev/compose-dev.yml`](dev/compose-dev.yml). Re-run
`make dev-up` to pick up code changes.

## Provisioning the Pi

The server runs on a Raspberry Pi 4 (headless Ubuntu Server 24.04). To flash a
card that boots fully configured — static IP, key-only SSH, Docker installed —
see [`infra/pi/`](infra/pi/README.md):

```sh
make flash DEVICE=/dev/sdX
```
