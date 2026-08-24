# cursor-usage-widget

Unofficial Linux overlay that shows remaining Cursor plan usage: included dollars left, when the cycle resets, and two spend meters (Auto + Composer vs other models).

It is **not affiliated with Cursor / Anysphere**. There is no public usage SDK; the widget reads the session already stored by the Cursor desktop app and polls Cursor’s dashboard API. That API can change or return **403** (expired session, or a blocked/Cloudflare path — common without a working route to `api2.cursor.sh`).

Transparent, undecorated, always-on-top, skipped from the taskbar. Clicks pass through except the close button and the drag control.

## Requirements

- Linux with **X11 or XWayland** (`DISPLAY` set). Pure Wayland without XWayland cannot hit-test close/drag while click-through is on.
- [Go 1.25+](https://go.dev/dl/)
- Cursor signed in on this machine (tokens live in `~/.config/Cursor/User/globalStorage/state.vscdb`)

## Run

```bash
go run .
```

| Flag | Default | |
| --- | --- | --- |
| `-poll` | `90s` | Refresh interval |
| `-once` | off | Print a usage JSON snapshot and exit (no window) |

```bash
go run . -once
go run . -poll 30s
```

**In the window:** drag the four-way pad (left of close) to move. Position is saved under `~/.config/cursor-usage-widget/state.json`. `R` refreshes. Close, `Esc`, or `Q` quits.

## Build

```bash
go build -o cursor-usage-widget .
```

Put the binary on `PATH` if you want the app-menu launcher (`Exec=cursor-usage-widget`).

## App menu (Linux)

A symlink in `/usr/bin` does not give an icon. Use a `.desktop` file:

```bash
chmod +x install-desktop.sh
./install-desktop.sh
```

That installs user-local files under `~/.local/share/applications/` and `~/.local/share/icons/hicolor/`. Kill the old process and start `cursor-usage-widget` again. Log out if the grid does not refresh.

System-wide (binary already on `PATH`):

```bash
sudo install -Dm644 assets/cursor-remaining.desktop /usr/share/applications/cursor-remaining.desktop
sudo install -Dm644 assets/icon-48.png /usr/share/icons/hicolor/48x48/apps/cursor-remaining.png
sudo install -Dm644 assets/icon.png /usr/share/icons/hicolor/128x128/apps/cursor-remaining.png
sudo install -Dm644 assets/icon-256.png /usr/share/icons/hicolor/256x256/apps/cursor-remaining.png
sudo update-desktop-database /usr/share/applications
sudo gtk-update-icon-cache -f /usr/share/icons/hicolor
```

## Privacy

The widget never sends your token anywhere except Cursor’s own API, using the same local session as the desktop app. Do not commit `state.vscdb`, `state.json`, or binaries.

## Layout

| Path | Role |
| --- | --- |
| `main.go` | Flags, window policy, `-once` |
| `internal/cursor` | SQLite auth + dashboard HTTP |
| `internal/ui` | Overlay, click-through, drag, draw |
| `assets/` | Icon + `.desktop` |
