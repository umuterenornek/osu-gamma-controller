# osu-gamma-controller

Automatically changes your screen's gamma (brightness) based on the AR (approach rate) of the beatmap
you're playing in osu!. For example, it can darken the screen for low-AR maps and brighten it for
high-AR maps. Which gamma is used for which AR is set in `config.json`.

It reads the current beatmap from [tosu](https://github.com/tosuapp/tosu), so tosu must be running
alongside osu!. Your normal gamma is restored when you close the program, and while osu!/tosu isn't
running.

> This is an unofficial fan-made tool. It is not affiliated with or endorsed by ppy Pty Ltd, the
> makers of osu!. "osu!" is a trademark of ppy Pty Ltd.

## Installing and using (Windows)

### 1. Download

1. Go to the [latest release page](https://github.com/umuterenornek/osu-gamma-controller/releases/latest).
2. Under **Assets**, click **`osu-gamma-controller-windows-amd64.zip`** to download it.
   (Only pick the `arm64` version if your PC has an ARM processor, such as a Snapdragon laptop.)
3. Open your Downloads folder, right-click the zip file and choose **Extract All...**, then click
   **Extract**. Move the extracted folder somewhere you'll find it again, such as your Desktop or
   Documents.

The folder contains:

| File | What it is |
| --- | --- |
| `osu-gamma-controller.exe` | The program |
| `config.json` | Your settings: which AR gets which gamma |
| `enable-full-gamma-range.reg` | A one-time Windows setting, see the next step |

### 2. Allow strong gamma values (one time only)

By default Windows refuses gamma values that are far from normal, so values like `0.3` (very dark) or
`6.5` (very bright) from the default settings won't work. To allow them:

1. Double-click **`enable-full-gamma-range.reg`**.
2. Click **Yes** when Windows asks for permission, and **Yes** again to confirm adding the setting.
   You should see a message saying the keys and values were added successfully.
3. **Restart your computer.** The setting does not take effect until you restart.

<details>
<summary>Prefer to do it by typing a command instead?</summary>

Open the Start menu, type `cmd`, right-click **Command Prompt** and choose **Run as administrator**.
Paste this command and press Enter:

```bat
reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ICM" /v GdiIcmGammaRange /t REG_DWORD /d 256 /f
```

It should print `The operation completed successfully.` Then **restart your computer**. The setting
does not take effect until you restart.

</details>

You can skip this step if you only use gamma values close to `1`.

### 3. Run it

1. Start [tosu](https://github.com/tosuapp/tosu) and osu! (in any order).
2. Double-click **`osu-gamma-controller.exe`**. A black console window opens and shows what the
   program is doing. Leave it open (you can minimize it) while you play.

The first time you run it, Windows may show **"Windows protected your PC"**. This happens with any
program that isn't from a big, paid-for publisher. Click **More info**, then **Run anyway**.

If osu! or tosu isn't running yet, the window will say it's retrying every 10 seconds. That's normal;
it connects on its own once tosu is up.

To stop the program, close the console window (or click it and press `Ctrl+C`). Your normal gamma is
restored.

### 4. Change the settings (optional)

Right-click `config.json`, choose **Open with** and pick **Notepad**. Each entry says "for AR from
`min` to `max`, use gamma `value`":

```json
{ "min": 10.0, "max": 10.2, "value": 1.3 },
```

Gamma `1` is normal. Values above `1` brighten the screen and values below `1` darken it. Save the
file and restart `osu-gamma-controller.exe` for the changes to apply. Keep the commas and brackets as
they are; if the program shows an error about the config after a change, there's probably a typo in
the file.

### Troubleshooting

- **The window says "Press Enter to close this window...".** The program couldn't start. The line
  above it explains why, for example a typo in `config.json` or a missing `config.json` next to the
  `.exe`.
- **The gamma doesn't change, or strong values like `0.3` or `6.5` don't work.** Make sure you did
  [step 2](#2-allow-strong-gamma-values-one-time-only) **and restarted your computer** afterwards.
- **The screen stays dark or bright after the program closed.** Start it again and close it
  normally, or restart your computer.

## Installing and using (Linux)

1. Download `osu-gamma-controller-linux-amd64.tar.gz` (or `-arm64` on ARM) from the
   [latest release page](https://github.com/umuterenornek/osu-gamma-controller/releases/latest).
2. Extract it and run it from a terminal, with tosu and osu! running:

   ```sh
   tar -xzf osu-gamma-controller-linux-amd64.tar.gz
   cd osu-gamma-controller-linux-amd64
   ./osu-gamma-controller
   ```

   Press `Ctrl+C` to stop it. To use a config file somewhere else, pass its path:
   `./osu-gamma-controller ~/.config/osu-gamma.json`.

The right way to change gamma is detected automatically (see [Supported platforms](#supported-platforms)).
Change the AR-to-gamma settings by editing `config.json`, as described in the
[Windows section](#4-change-the-settings-optional).

## Supported platforms

The backend is picked automatically. To force one, set `"backend"` in `config.json`.

| Platform | Backend | How it works |
| --- | --- | --- |
| Wayland: Sway, Hyprland, niri, river, Wayfire, labwc, other wlroots-based compositors | `wlr` | `wlr-gamma-control-unstable-v1` protocol. The compositor restores gamma automatically if the program exits or crashes. |
| Wayland: KDE Plasma 6 | `kwin` | Generates an ICC profile with a VCGT gamma curve and assigns it with `kscreen-doctor`. The original profile settings are restored on exit, or on the next start after a crash. |
| X11 | `x11` | RandR CRTC gamma ramps. |
| Windows | `gdi` | `SetDeviceGammaRamp` on every attached display. |

GNOME on Wayland is not supported: Mutter gives applications no way to change gamma.

Gamma values follow `xgamma` semantics: values above 1 brighten, values below 1 darken.

### Notes

- **wlr:** only one program can control gamma on an output at a time. Stop gammastep, wlsunset,
  hyprsunset or similar tools first.
- **KDE Plasma:** during gameplay each output's color profile is switched to the generated
  sRGB-based ICC profile, which replaces any EDID or custom ICC profile you use. HDR outputs are not
  supported.
- **Windows:** without the `GdiIcmGammaRange` registry value (see
  [step 2](#2-allow-strong-gamma-values-one-time-only)), Windows rejects gamma ramps that differ much
  from the default. The value only takes effect after a reboot.

## Building from source

Requires Go (see `go.mod` for the version). No cgo or system libraries are needed.

```sh
go build
./osu-gamma-controller [config.json]
```

Cross-compile by setting `GOOS`/`GOARCH`, e.g. `GOOS=windows GOARCH=amd64 go build`. Platform-specific
code lives in `*_linux.go` / `*_windows.go` files, so each binary only contains its own backends.

### Releases

[`.github/workflows/release.yml`](.github/workflows/release.yml) builds `windows-{amd64,arm64}` and
`linux-{amd64,arm64}` on every push and pull request, and uploads the archives as workflow artifacts.
Pushing a `v*` tag also publishes them as a GitHub release, with a `SHA256SUMS.txt`:

```sh
git tag v1.0.0
git push origin v1.0.0
```

Each archive contains only the stripped binary and `config.json`, plus the `.reg` file on Windows.
