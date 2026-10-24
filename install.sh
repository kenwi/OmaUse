#!/bin/sh
# Install OmaUse for the current user and add a floating launcher.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
bindir=${HOME}/.local/bin
appdir=${HOME}/.local/share/applications
bin=${bindir}/omause

if ! command -v go >/dev/null 2>&1; then
	echo "go is required to build OmaUse" >&2
	exit 1
fi
if ! command -v foot >/dev/null 2>&1; then
	echo "foot is required to open OmaUse" >&2
	exit 1
fi

mkdir -p "$bindir" "$appdir"
go build -C "$root" -o "$bin" ./cmd/omause

install -m 755 "$root/omause-launch" "${bindir}/omause-launch"

cat > "${appdir}/omause.desktop" << 'EOF'
[Desktop Entry]
Version=1.0
Type=Application
Name=OmaUse
Comment=Disk and ZFS pool usage
Exec=omause-launch
Icon=drive-harddisk
Terminal=false
Categories=System;Monitor;
StartupNotify=true
StartupWMClass=omause
EOF

if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database "$appdir" >/dev/null 2>&1 || true
fi

echo "Installed ${bin}"
echo "Launcher: OmaUse"
