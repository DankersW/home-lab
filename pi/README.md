# Raspberry Pi 4 — headless Ubuntu Server 24.04

Flash an SD card with Ubuntu Server 24.04 LTS for a Raspberry Pi 4 and have it
boot fully provisioned and headless via cloud-init: hostname `homelab`, user
`wouter` (key-only SSH, passwordless sudo), a static IP, mDNS (`homelab.local`),
and Docker CE + the compose plugin installed and enabled. No stack is deployed
automatically — you deploy it yourself after first boot (see below).

## What's in here

| File             | Purpose                                                  |
| ---------------- | -------------------------------------------------------- |
| `user-data`      | cloud-config: user, SSH key, packages, Docker install    |
| `network-config` | netplan v2: static `192.168.1.10/24`, gw `192.168.1.1`   |
| `meta-data`      | NoCloud seed: instance-id + hostname                     |
| `flash.sh`       | download, verify, write the image, copy the seed files   |

## Host prerequisites

The flashing machine (Linux) needs `xz-utils` (`xz`), `curl`, and the standard
disk tools from `util-linux` (`lsblk`, `findmnt`, `blockdev`), `parted`
(`partprobe`) and `udev` (`udevadm`) — plus `sudo` for `dd` and mounting.

```sh
sudo apt install xz-utils curl util-linux parted
```

## Flash (recommended: scripted)

From the repo root, pass the WHOLE-disk device (never a partition):

```sh
make flash DEVICE=/dev/sdX
```

Identify the card first with `lsblk`. The script downloads the image into `pi/`
if absent, verifies it against the official `SHA256SUMS` (and a pinned hash),
then prints the target and makes you type the device path to confirm before
writing. Everything on the device is erased.

## Flash (alternative: Raspberry Pi Imager GUI)

Use **Imager 2.x (>= 2.0.6)** — older 1.x builds silently ignore cloud-init
seeding. Choose `Other general-purpose OS` → `Ubuntu` → `Ubuntu Server 24.04.x
LTS (64-bit)`; Imager downloads and verifies the image for you.

**Caveat — pick ONE source of truth:** if you use Imager's OS-customization
(gear) dialog it writes its *own* `user-data`/`network-config` onto
`system-boot`, which conflicts with the files here — only one wins. Either drive
the whole setup through Imager's GUI, OR choose `No, clear settings` in Imager
and copy `user-data`, `meta-data`, `network-config` onto the `system-boot`
partition manually (remove/reinsert the card so it remounts, then copy to the
partition root).

## First boot

Insert the card, connect Ethernet, power on. cloud-init provisions on the
**first boot only**; allow roughly 1–3 minutes (it installs avahi and Docker
over the network). Then:

```sh
ssh wouter@homelab.local      # mDNS
ssh wouter@192.168.1.10       # static IP
```

Login is key-only; no password is set. The first SSH session is a fresh login,
so `docker` works without `sudo` immediately. You may then patch the OS:

```sh
sudo apt update && sudo apt full-upgrade -y
```

## Deploy the stack (on the Pi)

cloud-init installs Docker but deploys nothing. To bring up the home-lab stack:

```sh
git clone <this-repo-url> ~/home-lab
cd ~/home-lab
make bootstrap                 # creates dirs + generates secrets
# put your Cloudflare Tunnel token in secrets/cloudflared_token
make up                        # docker compose up -d --build
```

Useful afterwards: `make logs`, `make stop`, `make backup`.

## Troubleshooting

- **No SSH after a few minutes?** Console in (keyboard + HDMI) and run
  `cloud-init status --long` (or `--wait` to block until done). Logs:
  `/var/log/cloud-init.log`, `journalctl -u cloud-init`.
- **`homelab.local` won't resolve but `192.168.1.10` works?** avahi may still
  be starting, or your network blocks multicast (Wi-Fi client isolation / IGMP
  snooping). mDNS only works within the same subnet.
- **Wrong/no IP?** Confirm the wired interface is `eth0` with `ip link show` — a
  wrong name makes `network-config` silently fall back to DHCP. Keep networking
  in `network-config` (a separate file), never inside `user-data`, on 24.04.
- **Re-test provisioning?** `sudo cloud-init clean --logs --reboot` re-runs it;
  re-flash the card for a guaranteed clean state.

## Pinning note

`flash.sh` pins Ubuntu **24.04.4**
(SHA256 `790652faeb4f61ce7bb12f5cb61734595c61d3cd882915b8b5f9918106c80d37`).
When a newer point release lands, update `IMAGE_NAME` and `EXPECTED_SHA256` in
`flash.sh`.
