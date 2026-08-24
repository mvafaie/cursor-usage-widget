#!/usr/bin/env bash
# User-local launcher + icon (no sudo). Binary must be on PATH (e.g. /usr/bin).
set -euo pipefail

root=$(cd "$(dirname "$0")" && pwd)
data="${XDG_DATA_HOME:-$HOME/.local/share}"
appdir="$data/applications"
hicolor="$data/icons/hicolor"

mkdir -p "$appdir" \
	"$hicolor/48x48/apps" \
	"$hicolor/128x128/apps" \
	"$hicolor/256x256/apps"

install -m 644 "$root/assets/cursor-remaining.desktop" \
	"$appdir/cursor-remaining.desktop"
install -m 644 "$root/assets/icon.png" \
	"$hicolor/128x128/apps/cursor-remaining.png"

if [[ -f "$root/assets/icon-48.png" ]]; then
	install -m 644 "$root/assets/icon-48.png" \
		"$hicolor/48x48/apps/cursor-remaining.png"
else
	install -m 644 "$root/assets/icon.png" \
		"$hicolor/48x48/apps/cursor-remaining.png"
fi
if [[ -f "$root/assets/icon-256.png" ]]; then
	install -m 644 "$root/assets/icon-256.png" \
		"$hicolor/256x256/apps/cursor-remaining.png"
else
	install -m 644 "$root/assets/icon.png" \
		"$hicolor/256x256/apps/cursor-remaining.png"
fi

if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database "$appdir" || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
	gtk-update-icon-cache -f "$hicolor" 2>/dev/null || true
fi

echo "Installed $appdir/cursor-remaining.desktop"
echo "Kill the running widget and start cursor-usage-widget again."
echo "The Apps grid may need update-desktop-database (already tried) or a log out."
