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

## Provisioning the Pi

The server runs on a Raspberry Pi 4 (headless Ubuntu Server 24.04). To flash a
card that boots fully configured — static IP, key-only SSH, Docker installed —
see [`infra/pi/`](infra/pi/README.md):

```sh
make flash DEVICE=/dev/sdX
```
