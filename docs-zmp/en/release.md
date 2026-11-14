# Making a release (maintainers)

**English** | [Español](../es/release.md)

A GitHub Release of ZMP has these files, all built from the same commit of this repository:

| File | What | Made by |
|---|---|---|
| `ZMP-<version>-windows.zip` | the desktop game (soh.exe, soh.o2r, the ROM converter's descriptions, `zmp-server.exe` for the Host button, `zmp-invite.exe`, PLAYING.txt / JUGAR.txt, licenses) | `tools/package_windows.py` with `--server` |
| `ZMP-<version>-web.zip` | the browser version (index.html, soh.js, soh.wasm, soh.data, the in-browser ROM converter, favicon, icons, `app.webmanifest`, licenses; no `zmp-config.json`) | `web/make_web.py`, then zip the folder |
| `zmp-server-<version>-windows-amd64.exe`, `-linux-amd64`, `-linux-arm64` | the room server (the Windows one carries the ZMP icon) | `tools/build_server.py` |
| `zmp-invite-<version>-...` (the same three systems) | the invite code tool | `tools/build_server.py` |
| `*.sha256.txt` | checksums (and the soh.exe hash every player of a room must share) | the same scripts |

**Never** attach a ROM, an `oot.o2r` / `oot-mq.o2r`, or anything extracted from a ROM. Every script runs
`tools/check_release.py` on what it makes; run it again on every file just before uploading:

```powershell
Get-ChildItem dist\*.zip | ForEach-Object { python tools\check_release.py $_.FullName }
```

## Where to build

Build the release on the CI (`.github/workflows/ci.yml` uploads the Windows zip as an artifact) or on a PC with a
**fresh vcpkg and no binary cache** (`$env:VCPKG_BINARY_SOURCES = "clear"`, `VCPKG_ROOT` outside your user folder).
Libraries taken from a vcpkg cache keep the absolute source paths of the machine that compiled them inside
`soh.exe`, which can include a Windows user name; `check_release.py` refuses such a file.

## Steps

1. Make sure `go test ./...` passes in `server/`.
2. If the icon changed, rebuild the server's Windows icon resource (it is committed, so this is rarely needed):
   `python tools\make_icon_syso.py branding-zmp\icono-zmp.ico server\cmd\zmp-server\rsrc_windows_amd64.syso`.
   `go build` links it into `zmp-server.exe` by itself.
3. `python tools\build_server.py --version <version>`: the tests, then `zmp-server` and `zmp-invite` for every
   system, in `dist\`.
4. Build the Windows game with `-DZMP_HARNESS=OFF` ([build.md](build.md)) and package it with the server and the
   code tool inside: `python tools\package_windows.py --build <cpack zip> --version <version> --server ...`
   (`--help` lists the exact arguments). Check that the zip has `zmp-server.exe` and `zmp-invite.exe` next to
   `soh.exe`: without them the Host button says the server is missing.
5. Build the browser game ([build.md](build.md)), then
   `python web\make_web.py --build build\web\soh --out dist\web` (it adds the favicon, the icons and
   `app.webmanifest`) and zip the **contents** of `dist\web` as `dist\ZMP-<version>-web.zip`. Note the printed build
   hash in the release notes.
6. Run `tools\check_release.py` on every zip (above). Each must say `clean`.
7. Smoke test before publishing, everything on `127.0.0.1`: two desktop instances from the zip (with your own ROM),
   one presses **Host and play** with "This PC only", the other joins with the code; two browser tabs served by
   `zmp-server -web dist\web` join a room.
8. Tag the commit (`git tag v<version>`), push the tag, create the Release on GitHub from the tag, attach the files,
   and write the notes: what changed, the build hashes, whether rooms saved by the previous version still open, and
   that every player must update.

## Version rules

- A new game build (any change to the game's code) means a new release for everyone: rooms refuse mismatched builds.
- The server is compatible across game releases as long as the protocol number stays the same (`zmp-server -version`
  prints it); a protocol change is called out in the notes. A protocol 5 server (0.7.0) still lets protocol 4 games
  (0.6.x) into rooms without a password.
