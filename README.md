# osu-gamma-controller

Changes display gamma based on the current beatmap's AR, read from the [tosu](https://github.com/tosuapp/tosu)
websocket (`ws://127.0.0.1:24050/websocket/v2`). AR ranges map to gamma values in `config.json`.

```sh
go build
./osu-gamma-controller [config.json]
```

Gamma values follow `xgamma` semantics: values above 1 brighten, values below 1 darken. The original
gamma is restored on exit and while osu!/tosu is not connected.

## Supported platforms

The backend is picked automatically. To force one, set `"backend"` in `config.json`.

| Platform | Backend | How it works |
| --- | --- | --- |
| Wayland: Sway, Hyprland, niri, river, Wayfire, labwc, other wlroots-based compositors | `wlr` | `wlr-gamma-control-unstable-v1` protocol. The compositor restores gamma automatically if the program exits or crashes. |
| Wayland: KDE Plasma 6 | `kwin` | Generates an ICC profile with a VCGT gamma curve and assigns it with `kscreen-doctor`. The original profile settings are restored on exit, or on the next start after a crash. |
| X11 | `x11` | RandR CRTC gamma ramps. |
| Windows | `gdi` | `SetDeviceGammaRamp` on every attached display. |

GNOME on Wayland is not supported: Mutter gives applications no way to change gamma.

### Notes

- **wlr:** only one program can control gamma on an output at a time. Stop gammastep, wlsunset,
  hyprsunset or similar tools first.
- **KDE Plasma:** during gameplay each output's color profile is switched to the generated
  sRGB-based ICC profile, which replaces any EDID or custom ICC profile you use. HDR outputs are not
  supported.
- **Windows:** by default Windows rejects gamma ramps that differ much from the default, which rules out
  values like `0.3` or `6.5`. To allow the full range, run this in an elevated prompt and reboot:

  ```bat
  reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ICM" /v GdiIcmGammaRange /t REG_DWORD /d 256 /f
  ```
