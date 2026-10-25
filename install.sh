#!/bin/sh
# Install or uninstall OmaUse for the current user.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
bindir=${HOME}/.local/bin
appdir=${HOME}/.local/share/applications
bin=${bindir}/omause
launch=${bindir}/omause-launch
desktop=${appdir}/omause.desktop
bindings=${XDG_CONFIG_HOME:-$HOME/.config}/hypr/bindings.lua
configdir=${XDG_CONFIG_HOME:-$HOME/.config}/omause
configfile=$configdir/config.json
if [ -n "${XDG_STATE_HOME:-}" ]; then
	statedir=$XDG_STATE_HOME/omause
else
	statedir=$HOME/.local/state/omause
fi
uid=$(id -u)
cron_marker='# OmaUse quiet mode every minute'
hotkey_start='-- start omause --'
hotkey_end='-- end omause --'

usage() {
	cat << 'EOF'
Usage:
  ./install.sh              install OmaUse
  ./install.sh --uninstall  remove OmaUse
  ./install.sh -u           same as --uninstall
  ./install.sh --help       show this help
EOF
}

# ask prompt default(y|n) -> sets ask_reply to y or n
ask() {
	prompt=$1
	default=$2
	if [ ! -t 0 ]; then
		ask_reply=$default
		echo "$prompt [$default] (non-interactive: $default)"
		return
	fi
	case $default in
	y | Y) hint='Y/n' ;;
	*) hint='y/N' ;;
	esac
	while true; do
		printf '%s [%s] ' "$prompt" "$hint"
		read -r reply || reply=
		reply=$(printf '%s' "$reply" | tr '[:upper:]' '[:lower:]')
		case ${reply:-$default} in
		y | yes)
			ask_reply=y
			return
			;;
		n | no)
			ask_reply=n
			return
			;;
		esac
		echo "Please answer y or n."
	done
}

strip_hotkey() {
	[ -e "$bindings" ] || return 0
	tmp=$(mktemp)
	awk -v start="$hotkey_start" -v end="$hotkey_end" '
		$0 == start { skip = 1; next }
		skip && $0 == end { skip = 0; next }
		skip && /^-- start / { skip = 0; print; next }
		skip { next }
		{ print }
	' "$bindings" > "$tmp"
	awk 'NF { for (i = 1; i <= n; i++) print b[i]; n = 0; print; next } { b[++n] = $0 }' "$tmp" > "$bindings"
	rm -f "$tmp"
}

install_hotkey() {
	mkdir -p "$(dirname -- "$bindings")"
	if [ ! -e "$bindings" ]; then
		cat > "$bindings" << 'EOF'
-- Keep only your personal keybinding overrides here.
EOF
	fi
	strip_hotkey
	{
		cat "$bindings"
		printf '\n%s\n' "$hotkey_start"
		printf 'hl.unbind("SUPER + SHIFT + U")\n'
		printf 'o.bind("SUPER + SHIFT + U", "OmaUse", { launch = "omause-launch" })\n'
		printf '%s\n' "$hotkey_end"
	} > "${bindings}.tmp"
	mv "${bindings}.tmp" "$bindings"
	echo "Hotkey: SUPER + SHIFT + U -> omause-launch ($bindings)"
}

remove_hotkey() {
	if [ ! -e "$bindings" ]; then
		echo "Hotkey: none"
		return 0
	fi
	strip_hotkey
	echo "Removed hotkey from $bindings"
}

crontab_without_omause() {
	crontab -l 2>/dev/null | awk '
		/OmaUse quiet mode/ { skip = 1; next }
		skip && /omause -q/ { skip = 0; next }
		skip && /^[[:space:]]*$/ { next }
		skip { skip = 0 }
		/omause -q/ { next }
		{ print }
	' | awk 'NF { for (i = 1; i <= n; i++) print b[i]; n = 0; print; next } { b[++n] = $0 }'
}

install_crontab() {
	cron_line="* * * * * XDG_RUNTIME_DIR=/run/user/${uid} DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/${uid}/bus PATH=${HOME}/.local/bin:/usr/lib/flux/bin:/usr/bin:/bin flock -n /tmp/omause-quiet.lock ${bin} -q"
	{
		crontab_without_omause
		printf '\n%s\n%s\n' "$cron_marker" "$cron_line"
	} | crontab -
	echo "Crontab: omause -q every minute"
}

remove_crontab() {
	if ! crontab -l 2>/dev/null | grep -q 'omause -q'; then
		echo "Crontab: none"
		return 0
	fi
	cleaned=$(crontab_without_omause)
	if [ -z "$cleaned" ]; then
		crontab -r 2>/dev/null || true
	else
		printf '%s\n' "$cleaned" | crontab -
	fi
	echo "Removed omause crontab entry"
}

edit_config() {
	editor=${VISUAL:-${EDITOR:-}}
	if [ -z "$editor" ]; then
		if command -v nano >/dev/null 2>&1; then
			editor=nano
		elif command -v vi >/dev/null 2>&1; then
			editor=vi
		else
			echo "No editor found (set VISUAL or EDITOR)." >&2
			return 1
		fi
	fi
	echo "Opening $configfile with $editor"
	# Intentional word-splitting so EDITOR="code --wait" works.
	# shellcheck disable=SC2086
	$editor "$configfile"
}

do_install() {
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
	install -m 755 "$root/omause-launch" "$launch"

	mkdir -p "$configdir"
	if [ -e "$configfile" ]; then
		echo "Keeping existing config: $configfile"
	else
		install -m 644 "$root/config.example.json" "$configfile"
		echo "Config: $configfile"
	fi

	cat > "$desktop" << 'EOF'
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

	# Interactive installs default to yes; piped/non-TTY runs skip optionals.
	if [ -t 0 ]; then
		opt_default=y
	else
		opt_default=n
	fi

	ask "Install SUPER + SHIFT + U hotkey for omause-launch?" "$opt_default"
	if [ "$ask_reply" = y ]; then
		install_hotkey
	else
		echo "Skipped hotkey."
	fi

	ask "Install crontab to run omause -q every minute?" "$opt_default"
	if [ "$ask_reply" = y ]; then
		install_crontab
	else
		echo "Skipped crontab."
	fi

	ask "Edit quiet-mode config now?" "$opt_default"
	if [ "$ask_reply" = y ]; then
		edit_config
	else
		echo "Edit later: $configfile"
	fi
}

do_uninstall() {
	echo "Uninstalling OmaUse..."
	remove_hotkey
	remove_crontab

	rm -f "$bin" "$launch"
	echo "Removed binaries from $bindir"

	if [ -e "$desktop" ]; then
		rm -f "$desktop"
		if command -v update-desktop-database >/dev/null 2>&1; then
			update-desktop-database "$appdir" >/dev/null 2>&1 || true
		fi
		echo "Removed $desktop"
	fi

	if [ -e "$statedir" ]; then
		rm -rf "$statedir"
		echo "Removed state $statedir"
	else
		echo "State: none"
	fi
	rm -f /tmp/omause-quiet.lock

	if [ -e "$configfile" ] || [ -d "$configdir" ]; then
		ask "Also remove config $configfile?" n
		if [ "$ask_reply" = y ]; then
			rm -f "$configfile"
			rmdir "$configdir" 2>/dev/null || true
			echo "Removed config"
		else
			echo "Keeping config: $configfile"
		fi
	else
		echo "Config: none"
	fi

	echo "OmaUse uninstalled"
}

case ${1:-} in
'' )
	do_install
	;;
--uninstall | -u | uninstall)
	do_uninstall
	;;
-h | --help | help)
	usage
	;;
*)
	echo "omause install: unknown argument $1" >&2
	usage >&2
	exit 2
	;;
esac
