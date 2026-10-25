# OmaUse

A terminal view of mounted filesystems and ZFS pools. Each row shows the name, kind, mount path, total, used, free, percentage, a fullness bar, and pool health.

The bar and percentage are green under 70%, yellow from 70% to 89%, and red at 90% and above. The list refreshes every 2 seconds, and every column can be sorted.

![OmaUse showing disk and ZFS pool usage](screenshot.png)

## Install

```sh
./install.sh
```

This builds `omause` and installs `omause-launch` into `~/.local/bin`, and adds an **OmaUse** launcher in `~/.local/share/applications`. `omause-launch` measures the table, opens foot at that character size, and Hyprland floats and centers the OmaUse window.

`go` and `foot` are required. ZFS pools are included when `zpool` is available. If it is not, filesystems still show and a line says the pools could not be read.

## Run

```sh
omause
```

Or launch **OmaUse** from the app menu. A key can do the same by running `omause-launch`, which is what opens the floating window. In `~/.config/hypr/bindings.lua`:

```lua
hl.unbind("SUPER + SHIFT + U")
o.bind("SUPER + SHIFT + U", "OmaUse", { launch = "omause-launch" })
```

## Quiet mode

```sh
omause -q
omause -c       # reset fired alerts
omause -c -q    # reset, then check and notify
```


Collects drive and pool usage once, applies alerts from `~/.config/omause/config.json`, then exits (no TUI). Install copies an example config if that file is missing.

Each threshold notifies once via `omarchy-notification-send`. An optional `script` runs when that level is crossed. Fired levels are stored in `~/.local/state/omause/thresholds.json` and reset when usage falls below them, so the next crossing alerts again.

```json
{
  "enabled": true,
  "thresholds": [
    { "percent": 80 },
    { "percent": 90, "script": "/home/you/.config/omause/cleanup.sh" }
  ]
}
```

Set `"enabled": false` to keep thresholds defined but skip all alerts.

Scripts receive `OMAUSE_NAME`, `OMAUSE_WHERE`, `OMAUSE_USE_PCT`, `OMAUSE_THRESHOLD`, and size fields in the environment. A systemd user timer or cron entry can run `omause -q` on an interval.

## Keys

| Key | Action |
| --- | --- |
| Left / Right | Move the sort column |
| `s` | Reverse sort direction |
| Space | Pause or resume the timer |
| `+` / `-` | Change the refresh interval (1-60 seconds) |
| `r` | Refresh now |
| `q` | Quit |

Use% starts descending, so the fullest volume is first. Other columns start ascending. Sizes that cannot be read sort last.

## License

MIT. See [LICENSE](LICENSE).
