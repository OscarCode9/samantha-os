#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
INITIAL_SETUP_DIR="$ROOT_DIR/references/initial-setup"
SERVICE_SOURCE="$ROOT_DIR/deployments/systemd/elementary-claw.service"

echo "[1/6] Repairing and refreshing APT metadata in the guest..."
sudo rm -rf /var/lib/apt/lists/*
sudo apt-get clean
sudo apt-get update

echo "[2/6] Installing build dependencies and browser support in the guest..."
sudo apt-get install -y \
  build-essential \
  curl \
  desktop-file-utils \
  epiphany-browser \
  gettext \
  golang-go \
  jq \
  libaccountsservice-dev \
  libadwaita-1-dev \
  libgnomekbd-dev \
  libgranite-7-dev \
  libgtk-4-dev \
  libjson-glib-dev \
  libpantheon-wayland-1-dev \
  libpwquality-dev \
  libsoup-3.0-dev \
  libx11-dev \
  libxkbregistry-dev \
  meson \
  ninja-build \
  xdg-utils \
  valac

echo "[2.1/6] Registering the default browser for http(s)..."
sudo update-desktop-database /usr/share/applications >/dev/null 2>&1 || true

if command -v xdg-settings >/dev/null 2>&1; then
  xdg-settings set default-web-browser org.gnome.Epiphany.desktop >/dev/null 2>&1 || true
  xdg-settings set default-url-scheme-handler http org.gnome.Epiphany.desktop >/dev/null 2>&1 || true
  xdg-settings set default-url-scheme-handler https org.gnome.Epiphany.desktop >/dev/null 2>&1 || true
fi

if command -v xdg-mime >/dev/null 2>&1; then
  xdg-mime default org.gnome.Epiphany.desktop x-scheme-handler/http >/dev/null 2>&1 || true
  xdg-mime default org.gnome.Epiphany.desktop x-scheme-handler/https >/dev/null 2>&1 || true
  xdg-mime default org.gnome.Epiphany.desktop text/html >/dev/null 2>&1 || true
  xdg-mime default org.gnome.Epiphany.desktop application/xhtml+xml >/dev/null 2>&1 || true
fi

export GOTOOLCHAIN=auto

echo "[3/6] Building patched Initial Setup..."
cd "$INITIAL_SETUP_DIR"

if [[ -d build ]]; then
  meson setup build --reconfigure --prefix=/usr
else
  meson setup build --prefix=/usr
fi

ninja -C build

echo "[4/6] Installing patched Initial Setup..."
sudo meson install -C build
sudo fc-cache -f /usr/share/fonts/truetype/io.elementary.initial-setup || true

echo "[5/6] Building elementary-claw runtime..."
cd "$ROOT_DIR"
go version
go build -o claw ./cmd/claw

echo "[6/6] Installing elementary-claw runtime and user service..."
sudo install -d /usr/local/bin
sudo install -m 0755 claw /usr/local/bin/claw
sudo install -d /etc/systemd/user
sudo install -m 0644 "$SERVICE_SOURCE" /etc/systemd/user/elementary-claw.service
sudo systemctl --global enable elementary-claw.service

if systemctl --user daemon-reload >/dev/null 2>&1; then
  systemctl --user enable --now elementary-claw.service >/dev/null 2>&1 || true
fi

echo
echo "Provision complete."
echo "Next commands:"
echo "  io.elementary.initial-setup"
echo "  # if this VM was provisioned before the latest polkit rule, reinstall with:"
echo "  sudo meson install -C ~/elementary-claw-vm/references/initial-setup/build"
echo "  claw --help | head"
echo "  systemctl --user status elementary-claw --no-pager"
echo "  claw gateway status"
echo
echo "Detailed validation:"
echo "  $ROOT_DIR/initial-setup-vm-runbook.md"