# Home Lab

Self hosting open source alternative to expensive subscriptions

Auth is handled by cloudflare SSO Access

## Provisioning the Pi

The server runs on a Raspberry Pi 4 (headless Ubuntu Server 24.04). To flash a
card that boots fully configured — static IP, key-only SSH, Docker installed —
see [`pi/`](pi/README.md):

```sh
make flash DEVICE=/dev/sdX
```

Then `ssh wouter@homelab.local` and `make up` to bring the stack online.
