# cursor-usage-widget

A small floating window that shows your current Cursor plan usage: remaining included dollars, when the cycle resets, and two spend meters (Auto + Composer vs other models).

It reads the session already stored by the Cursor desktop app (no extra login) and polls `api2.cursor.sh`. Transparent, undecorated, always-on-top, skipped from the taskbar.

You need Cursor signed in on this machine. The widget looks up tokens in Cursor’s `state.vscdb` (`~/.config/Cursor/User/globalStorage/` on Linux).

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

**In the window:** clicks pass through except the close button and the 6-dot drag grip (left of close). Drag the grip to reposition; location is saved under `~/.config/cursor-usage-widget/state.json`. `R` refreshes immediately. Close, `Esc`, or `Q` quits.

Click-through hit-testing reads the pointer from X11 (`QueryPointer` on the root window). A pure Wayland session without XWayland (`DISPLAY` unset) cannot see the cursor over close/grip while passthrough is on — use X11 or XWayland.

## App menu icon

A symlink in `/usr/bin` has no icon. Linux Apps grids read a `.desktop` file plus a themed icon. `Exec=cursor-usage-widget` (so the binary must be on `PATH`). Kill the old process and start the new binary after installing.

**User-local (no sudo):**

```bash
chmod +x install-desktop.sh
./install-desktop.sh
```

That copies into `~/.local/share/applications/` and `~/.local/share/icons/hicolor/{48x48,128x128,256x256}/apps/cursor-remaining.png`, then runs `update-desktop-database` / `gtk-update-icon-cache` if those tools exist.

Manual equivalent:

```bash
mkdir -p ~/.local/share/applications \
  ~/.local/share/icons/hicolor/48x48/apps \
  ~/.local/share/icons/hicolor/128x128/apps \
  ~/.local/share/icons/hicolor/256x256/apps
cp assets/cursor-remaining.desktop ~/.local/share/applications/
cp assets/icon-48.png ~/.local/share/icons/hicolor/48x48/apps/cursor-remaining.png
cp assets/icon.png ~/.local/share/icons/hicolor/128x128/apps/cursor-remaining.png
cp assets/icon-256.png ~/.local/share/icons/hicolor/256x256/apps/cursor-remaining.png
update-desktop-database ~/.local/share/applications
gtk-update-icon-cache -f ~/.local/share/icons/hicolor
```

**System-wide** (binary already in `/usr/bin`):

```bash
sudo install -Dm644 assets/cursor-remaining.desktop /usr/share/applications/cursor-remaining.desktop
sudo install -Dm644 assets/icon-48.png /usr/share/icons/hicolor/48x48/apps/cursor-remaining.png
sudo install -Dm644 assets/icon.png /usr/share/icons/hicolor/128x128/apps/cursor-remaining.png
sudo install -Dm644 assets/icon-256.png /usr/share/icons/hicolor/256x256/apps/cursor-remaining.png
sudo update-desktop-database /usr/share/applications
sudo gtk-update-icon-cache -f /usr/share/icons/hicolor
```

Log out or wait for the menu to refresh if the icon does not appear immediately.

## Build

```bash
go build -o cursor-usage-widget .
```

Requires Go 1.25+. Ubuntu fonts are used when present; otherwise embedded Go fonts.

## Why the first pass was long

The Cursor dashboard API is not a public SDK. The first version had to **discover the contract**: which Connect-RPC paths exist, which JSON fields are cents vs dollars, how billing-cycle timestamps arrive, which headers the server actually checks, and how the local SQLite auth store is laid out.

That exploration left residue that a first working widget always accumulates:

- Parse structs with extra fields “in case we need them later” (`TotalSpend`, account `Email`).
- A `setHeaders` flag for dashboard vs OAuth, because Origin was being tried on every request while the API shape was still unknown.
- UI helpers that existed only to try layouts (`drawGlassCard` wrapping a one-line round-rect fill).
- Comments that restated the next line.
- A more cautious token-lock sequence than the actual refresh path required.
- A longer font-load path while checking which faces rendered at widget sizes.

That is normal first-pass cost: you keep scaffolding until you know what the window actually displays.

## Why it was shortened

This is a 380×196 overlay, not a client library. Every unused field, wrapper, and comment is something to keep in sync when Cursor changes the dashboard payload.

Once the UI contract was stable — remaining USD, reset text, two percentages, poll/refresh/close — the extra bits were deleted. What remains is what the window or `-once` JSON still uses.

## Code map

### `main.go`

Flags, first fetch for `-once`, then an Ebiten window: undecorated, floating, transparent, 30 TPS, restored position.

Nothing interesting was cut here. It stays thin on purpose — window policy lives in `internal/ui`, API policy in `internal/cursor`.

### `internal/ui/widget.go`

Game loop: quit, manual refresh, click-through except close + drag grip, poll-when-due, draw.

**Kept:** named layout constants (`winW`, `pad`, `closeX`, `meterW`, …) so Draw is not a pile of magic numbers. `Update` is four named steps (`quit` / refresh / `handleDrag` / `pollIfDue`) instead of one 80-line function — each step has a reason to exist, not a reason to be a package.

**Cut:** `drawGlassCard` (it only called `fillRoundRect`). Comments that duplicated the code. The rounded-rect mask cache stayed: Ebiten has no fill-round-rect primitive, and recreating the mask every frame is wasted work on a 30 TPS overlay.

### `internal/ui/fonts.go`

Load Ubuntu Regular/Bold from disk; fall back to Go fonts.

**Cut:** the longer load path (extra wrappers / stepwise source setup). One `fontSource(path, fallback)` is enough.

### `internal/cursor/client.go`

HTTP client: ensure a token, POST `GetCurrentPeriodUsage` + `GetPlanInfo`, map cents to dollars and percents onto `Snapshot`.

**Cut:** `TotalSpend` — the meters use `autoPercentUsed` / `apiPercentUsed`, remaining uses `remaining`. A dashboard bool on `setHeaders` — Origin is only required on the dashboard RPCs, so it is set in `postRPC`, not on OAuth refresh. The extra mutex choreography in `ensureToken` — lock, copy what you need, unlock, then refresh outside the lock.

**Kept in `Snapshot` even if the window ignores them:** plan name, price, spend/limit/bonus USD, display messages, `LimitHit`, `FetchedAt`. `-once` dumps the struct; those fields are cheap to keep once parsing already has them.

### `internal/cursor/auth.go`

Read `cursorAuth/accessToken`, `refreshToken`, and plan hint from Cursor’s SQLite state DB. JWT expiry check with a 60s skew so refresh happens before the access token dies.

**Cut:** `Email`. The widget never showed it and the API calls do not need it.

## What was deliberately kept

| Behavior | Why |
| --- | --- |
| 90s poll | Usage does not change every frame; 90s is enough without hammering the dashboard RPC. `R` / right-click covers “I just burned a request.” |
| `$X.XX remaining` + `resets in …` | That is the number you actually care about. Cycle end comes from the usage payload (plan info as fallback). |
| Two meters | Cursor splits included spend into Auto+Composer vs named/other models. One bar would hide which bucket is eating the plan. |
| Click-through + close + grip + saved position | Overlay must not steal clicks from apps beneath it. Close and the drag grip still work; last position is restored on launch. |
| `-once` JSON | Same fetch path as the window, for scripts and for checking the raw snapshot without a GUI. |
| Transparent floating window | The whole point: sit on the desktop, not in a browser tab. |

Auth refresh on 401/403, client version sniffed from a local Cursor `package.json`, and a 20s HTTP timeout stay because they are failure modes you will hit, not speculative features.


go build -o cursor-usage-widget .
 sudo rm /usr/bin/cursor-usage-widget && sudo ln -s /home/mrv/Projects/tools/cursor-usage-widget/cursor-usage-widget /usr/bin/cursor-usage-widget