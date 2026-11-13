# Making a release (maintainers)

A GitHub Release of ZMP has these files, all built from the same commit of this repository:

| File | What | Made by |
|---|---|---|
| `ZMP-<version>-windows.zip` | the desktop game (soh.exe, soh.o2r, the ROM converter's descriptions, guides, licenses) | `tools/package_windows.py` |
| `ZMP-<version>-web.zip` | the browser version (index.html, soh.js, soh.wasm, soh.data, the in-browser ROM converter, licenses) | `web/make_web.py`, then zip the folder |
| `zmp-server-<version>-windows-amd64.exe`, `-linux-amd64`, `-linux-arm64` | the room server | `tools/build_server.py` |
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
2. Build the Windows game with `-DZMP_HARNESS=OFF` and package it ([build.md](build.md)):
   `python tools\package_windows.py --build <cpack zip> --version <version>`.
3. Build the browser game ([build.md](build.md)), then
   `python web\make_web.py --build build\web\soh --out dist\web` and zip the **contents** of `dist\web` as
   `dist\ZMP-<version>-web.zip`. Note the printed build hash in the release notes.
4. `python tools\build_server.py --version <version>`.
5. Run `tools\check_release.py` on every zip (above). Each must say `clean`.
6. Smoke test before publishing: two desktop instances from the zip (with your own ROM) join a room on a local
   `zmp-server`; two browser tabs served by `zmp-server -web dist\web` join a room.
7. Tag the commit (`git tag v<version>`), push the tag, create the Release on GitHub from the tag, attach the files,
   and write the notes: what changed, the build hashes, whether rooms saved by the previous version still open, and
   that every player must update.

## Version rules

- A new game build (any change to the game's code) means a new release for everyone: rooms refuse mismatched builds.
- The server is compatible across game releases as long as the protocol number stays the same (`zmp-server -version`
  prints it); a protocol change is called out in the notes.
