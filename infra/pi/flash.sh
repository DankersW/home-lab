#!/usr/bin/env bash
#
# flash.sh — write Ubuntu Server 24.04 for Raspberry Pi 4 to an SD card and
# seed it with this directory's cloud-init files (user-data, meta-data,
# network-config) for a headless, plug-and-go first boot.
#
set -euo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
readonly IMAGE_NAME="ubuntu-24.04.4-preinstalled-server-arm64+raspi.img.xz"
readonly IMAGE_PATH="${SCRIPT_DIR}/${IMAGE_NAME}"
readonly RELEASE_BASE="https://cdimage.ubuntu.com/releases/24.04/release"
readonly IMAGE_URL="${RELEASE_BASE}/${IMAGE_NAME}"
readonly CHECKSUM_URL="${RELEASE_BASE}/SHA256SUMS"
readonly CHECKSUM_PATH="${SCRIPT_DIR}/SHA256SUMS"
readonly EXPECTED_SHA256="790652faeb4f61ce7bb12f5cb61734595c61d3cd882915b8b5f9918106c80d37"
readonly MAX_DEVICE_BYTES=$((256 * 1024 * 1024 * 1024)) # refuse targets > 256GB
readonly SEED_FILES=(user-data meta-data network-config)

usage() {
  cat <<'EOF'
Usage: flash.sh DEVICE
       DEVICE=/dev/sdX ./flash.sh

Writes Ubuntu Server 24.04 (Raspberry Pi 4, arm64) to DEVICE and copies the
cloud-init seed files from this directory onto the system-boot partition.

DEVICE must be the WHOLE disk (e.g. /dev/sdb or /dev/mmcblk0), never a
partition (/dev/sdb1). EVERYTHING ON THE DEVICE IS ERASED.

Steps performed:
  1. Download the image into this directory if absent.
  2. Verify it against the official SHA256SUMS (fails closed on mismatch).
  3. Stream-decompress and dd it to DEVICE with safety guards + confirmation.
  4. Mount system-boot and copy user-data / meta-data / network-config.
EOF
}

die() {
  echo "error: $*" >&2
  exit 1
}

require_tools() {
  local missing=()
  local tool
  for tool in curl xz dd sha256sum lsblk partprobe udevadm findmnt mount umount; do
    command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
  done
  [ "${#missing[@]}" -eq 0 ] || die "missing required tools: ${missing[*]}"
}

download_image() {
  if [ -f "$IMAGE_PATH" ]; then
    echo "Image already present: $IMAGE_PATH"
    return
  fi
  echo "Downloading $IMAGE_NAME ..."
  curl -fL --output "$IMAGE_PATH" "$IMAGE_URL"
}

verify_image() {
  echo "Downloading checksums ..."
  curl -fL --output "$CHECKSUM_PATH" "$CHECKSUM_URL"

  echo "Verifying SHA256 ..."
  (cd "$SCRIPT_DIR" && sha256sum --ignore-missing -c SHA256SUMS) \
    || die "checksum verification failed; refusing to flash"

  # Defence in depth: pin the exact known-good hash, so a tampered SHA256SUMS
  # served alongside a tampered image still fails closed.
  local actual
  actual="$(sha256sum "$IMAGE_PATH" | awk '{print $1}')"
  [ "$actual" = "$EXPECTED_SHA256" ] || die "image hash mismatch (got $actual)"
  echo "Checksum OK."
}

resolve_root_disk() {
  local root_src
  root_src="$(findmnt -no SOURCE /)"
  echo "/dev/$(lsblk -no PKNAME "$root_src")"
}

validate_device() {
  local device="$1"

  [ -n "$device" ] || { usage; die "no DEVICE given"; }
  [ -b "$device" ] || die "$device is not a block device"

  if [[ "$device" =~ (mmcblk[0-9]+p[0-9]+|nvme[0-9]+n[0-9]+p[0-9]+|sd[a-z][0-9]+)$ ]]; then
    die "$device looks like a partition; pass the whole disk"
  fi
  [ "$(lsblk -ndo TYPE "$device")" = "disk" ] || die "$device is not a whole disk"

  local root_disk
  root_disk="$(resolve_root_disk)"
  [ "$device" != "$root_disk" ] || die "$device is the running system disk"

  # Read size from sysfs via lsblk (no privilege needed); blockdev would
  # require opening the raw device, which fails for non-root users.
  local size
  size="$(lsblk --bytes -dno SIZE "$device")"
  [ -n "$size" ] || die "could not determine size of $device"
  [ "$size" -le "$MAX_DEVICE_BYTES" ] || die "$device is larger than 256GB; refusing as likely not an SD card"

  local removable hotplug
  removable="$(lsblk -ndo RM "$device")"
  hotplug="$(lsblk -ndo HOTPLUG "$device")"
  if [ "$removable" != "1" ] && [ "$hotplug" != "1" ]; then
    echo "WARNING: $device is not removable/hotplug media."
  fi
}

confirm_device() {
  local device="$1"
  echo
  echo "Target device:"
  lsblk -o NAME,SIZE,MODEL,TRAN,RM,MOUNTPOINT "$device"
  echo
  echo "ALL DATA ON $device WILL BE DESTROYED."
  local answer
  read -rp "Type the device path ($device) to confirm: " answer
  [ "$answer" = "$device" ] || die "aborted by user"
}

unmount_device() {
  local device="$1"
  local part
  while read -r part; do
    [ -n "$part" ] || continue
    sudo umount "/dev/$part" 2>/dev/null || true
  done < <(lsblk -nro NAME "$device" | tail -n +2)
}

write_image() {
  local device="$1"
  echo "Writing image to $device (this takes a few minutes) ..."
  xz -dc "$IMAGE_PATH" | sudo dd of="$device" bs=4M conv=fsync status=progress
  sync
  echo "Image written."
}

# Resolve the system-boot partition strictly from the target disk, so a second
# card with the same label (common when flashing several) can never receive the
# SSH key / network config by mistake.
find_boot_partition() {
  local device="$1"
  local name label
  while read -r name label; do
    [ "$label" = "system-boot" ] && { echo "/dev/$name"; return 0; }
  done < <(lsblk -nro NAME,LABEL "$device")
  return 1
}

copy_seed_files() {
  local device="$1"

  echo "Re-reading partition table ..."
  sudo partprobe "$device"
  sudo udevadm settle

  local boot_part mount_point
  boot_part="$(find_boot_partition "$device")" \
    || die "could not find a system-boot partition on $device"

  mount_point="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "sudo umount '$mount_point' 2>/dev/null || true; rmdir '$mount_point' 2>/dev/null || true" RETURN

  sudo mount "$boot_part" "$mount_point"

  echo "Copying cloud-init seed files to $boot_part ..."
  local file
  for file in "${SEED_FILES[@]}"; do
    [ -f "${SCRIPT_DIR}/${file}" ] || die "missing seed file: ${SCRIPT_DIR}/${file}"
    sudo cp "${SCRIPT_DIR}/${file}" "${mount_point}/${file}"
  done

  ls -l "$mount_point"
  sync
}

main() {
  case "${1:-}" in
    -h | --help)
      usage
      exit 0
      ;;
  esac

  local device="${1:-${DEVICE:-}}"

  require_tools
  validate_device "$device"
  confirm_device "$device"

  download_image
  verify_image

  unmount_device "$device"
  write_image "$device"
  copy_seed_files "$device"

  echo
  echo "Done. Insert the card into the Pi and power on."
  echo "First boot provisions via cloud-init; reach it at:"
  echo "  ssh wouter@homelab.local   (or ssh wouter@192.168.1.10)"
}

main "$@"
