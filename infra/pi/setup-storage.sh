#!/usr/bin/env bash
#
# setup-storage.sh — make an external SSD the storage backend for the home-lab.
# Wipes the target disk, creates one ext4 partition, mounts it persistently
# (fstab by UUID), moves Docker's data-root onto it, and points the deploy dir
# (~/home-lab) at it via a symlink.
#
# Run ON THE PI as your normal user:  ./setup-storage.sh /dev/sda
#
set -euo pipefail

readonly DEVICE="${1:-/dev/sda}"
readonly MOUNT="${MOUNT:-/mnt/ssd}"
readonly LABEL="${LABEL:-homelab}"
readonly DOCKER_ROOT="$MOUNT/docker"
readonly DEPLOY_DIR="$MOUNT/home-lab"

die() { echo "error: $*" >&2; exit 1; }

require_tools() {
  local t missing=()
  for t in lsblk findmnt udevadm mountpoint python3 docker; do
    command -v "$t" >/dev/null 2>&1 || missing+=("$t")
  done
  [ "${#missing[@]}" -eq 0 ] || die "missing tools: ${missing[*]}"
}

validate_device() {
  [ -b "$DEVICE" ] || die "$DEVICE is not a block device"
  [ "$(lsblk -ndo TYPE "$DEVICE")" = "disk" ] || die "$DEVICE is not a whole disk"
  local root_disk
  root_disk="/dev/$(lsblk -no PKNAME "$(findmnt -no SOURCE /)")"
  [ "$DEVICE" != "$root_disk" ] || die "$DEVICE is the running system disk ($root_disk)"
}

confirm() {
  echo "Target SSD:"
  lsblk -o NAME,SIZE,TRAN,FSTYPE,LABEL,MOUNTPOINTS "$DEVICE"
  echo
  echo "This will WIPE $DEVICE, format it ext4 (label '$LABEL'), mount it at $MOUNT,"
  echo "move Docker's data-root to $DOCKER_ROOT, and link ~/home-lab -> $DEPLOY_DIR."
  local ans
  read -rp "Type the device path ($DEVICE) to confirm: " ans
  [ "$ans" = "$DEVICE" ] || die "aborted by user"
}

format_disk() {
  local p
  while read -r p; do [ -n "$p" ] && { sudo umount "/dev/$p" 2>/dev/null || true; }; done \
    < <(lsblk -nro NAME "$DEVICE" | tail -n +2)
  echo "Partitioning $DEVICE (GPT, single Linux partition) ..."
  sudo wipefs -a "$DEVICE"
  sudo sfdisk "$DEVICE" <<'EOF'
label: gpt
,,L
EOF
  sudo udevadm settle
  PART="/dev/$(lsblk -nro NAME "$DEVICE" | tail -n +2 | head -1)"
  [ -b "$PART" ] || die "partition did not appear on $DEVICE"
  echo "Formatting $PART ext4 ..."
  sudo mkfs.ext4 -F -L "$LABEL" "$PART"
}

mount_persistent() {
  local uuid
  uuid="$(sudo blkid -s UUID -o value "$PART")"
  [ -n "$uuid" ] || die "could not read UUID of $PART"
  sudo mkdir -p "$MOUNT"
  # Drop any prior entry for this mount point, then add ours (idempotent).
  sudo sed -i "\#[[:space:]]$MOUNT[[:space:]]#d" /etc/fstab
  echo "UUID=$uuid  $MOUNT  ext4  defaults,noatime,nofail,x-systemd.device-timeout=10  0  2" \
    | sudo tee -a /etc/fstab >/dev/null
  sudo systemctl daemon-reload
  sudo mount "$MOUNT"
  mountpoint -q "$MOUNT" || die "failed to mount $MOUNT"
  sudo chown "$USER:$USER" "$MOUNT"
  echo "Mounted $PART at $MOUNT (fstab by UUID=$uuid)"
}

move_docker_root() {
  echo "Pointing Docker data-root at $DOCKER_ROOT ..."
  sudo mkdir -p /etc/docker "$DOCKER_ROOT"
  # Merge data-root into daemon.json without clobbering any existing settings.
  sudo python3 - "$DOCKER_ROOT" <<'PY'
import json, os, sys
path, root = "/etc/docker/daemon.json", sys.argv[1]
cfg = {}
if os.path.exists(path):
    try: cfg = json.load(open(path))
    except Exception: cfg = {}
cfg["data-root"] = root
with open(path, "w") as f:
    json.dump(cfg, f, indent=2); f.write("\n")
PY
  sudo systemctl restart docker
  local actual
  actual="$(docker info --format '{{.DockerRootDir}}' 2>/dev/null || true)"
  [ "$actual" = "$DOCKER_ROOT" ] && echo "Docker data-root is now $actual" \
    || echo "WARNING: Docker data-root is '$actual' (expected $DOCKER_ROOT)"
}

link_deploy_dir() {
  sudo install -d -o "$USER" -g "$USER" "$DEPLOY_DIR"
  if [ -e "$HOME/home-lab" ] && [ ! -L "$HOME/home-lab" ]; then
    die "$HOME/home-lab exists as a real directory; move its contents into $DEPLOY_DIR and remove it, then re-run"
  fi
  ln -sfn "$DEPLOY_DIR" "$HOME/home-lab"
  echo "Linked $HOME/home-lab -> $DEPLOY_DIR"
}

main() {
  [ "$(id -u)" -ne 0 ] || die "run as your normal user (it uses sudo where needed), not root"
  require_tools
  validate_device
  confirm
  format_disk
  mount_persistent
  move_docker_root
  link_deploy_dir
  echo
  echo "Done. SSD ready at $MOUNT; Docker and ~/home-lab now live on the SSD."
}

main "$@"
