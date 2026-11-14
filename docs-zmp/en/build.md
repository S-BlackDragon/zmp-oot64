# Building from source

**English** | [Español](../es/build.md)

Everything needed is in this repository:

| Folder | What | Build tool |
|---|---|---|
| root (`soh/`, `libultraship/`, `torch/`, `CMakeLists.txt`...) | the game: Ship of Harkinian with ZMP (`soh/soh/Zmp/`), libultraship and torch as plain folders | CMake + Visual Studio (Windows), Emscripten (browser) |
| `server/` | `zmp-server`, the room server | Go |
| `web/` | `make_web.py`: turns the browser build into a folder you can serve | Python 3 |
| `tools/` | `package_windows.py` (player zip), `build_server.py` (server binaries), `check_release.py` (no ROM data in a release) | Python 3 |

No ROM and no game data are needed to build; they are only needed to play.

Commands below run in **PowerShell** from the repository root unless they say otherwise.

## Windows game

Requirements (versions that are known to work):

- Visual Studio 2022 (Community is fine) with the **Desktop development with C++** workload (MSVC v143, Windows SDK);
- CMake 3.26 or newer, Git, Python 3.

CMake downloads and builds the libraries the game needs with **vcpkg** the first time (it clones vcpkg into
`$env:VCPKG_ROOT`, or into the build folder if that variable is not set). The first configuration takes a while;
later ones are quick. It also downloads a few pinned libraries (Dear ImGui and others) with FetchContent.

```powershell
$env:VCPKG_ROOT = "$PWD\vcpkg"     # optional: one vcpkg for all your builds
cmake -S . -B build\win -G "Visual Studio 17 2022" -T v143 -A x64 -DCMAKE_BUILD_TYPE:STRING=Release -DZMP_HARNESS=OFF
cmake --build build\win --target GenerateSohOtr --config Release
cmake --build build\win --config Release --parallel
```

- `-DZMP_HARNESS=OFF` is the player build. `ON` (the default) adds the test harness the project uses for its
  automated tests (a control port per game instance); players don't need it.
- The game ends up in `x64\Release\soh.exe`, and Ship of Harkinian's own data `soh.o2r` in
  `build\win\soh\soh.o2r` (a copy is also written to `soh.o2r` at the repository root; the browser build needs it).
- Full build: about 8 minutes on a 12-core PC after vcpkg is ready.

Package it like a release (no ROM data can end up in it: the script checks):

```powershell
cd build\win; cpack -G ZIP -C Release; cd ..\..
python tools\package_windows.py --build (Get-ChildItem _packages\*.zip | Select-Object -First 1).FullName --version 0.6.2
```

The result is `dist\ZMP-0.6.2-windows.zip`. Every player of a room needs the same `soh.exe`, so give your friends
this zip rather than letting each one build their own.

## Browser game

Requirements: the [Emscripten SDK](https://emscripten.org/docs/getting_started/downloads.html) (tested with 7.0.0),
Ninja, Python 3, and the Windows build's `soh.o2r` (run at least the `GenerateSohOtr` step above first: the browser
build packs the `soh.o2r` of the repository root).

```powershell
& <emsdk folder>\emsdk_env.ps1
emcmake cmake -S . -B build\web -G Ninja -DCMAKE_BUILD_TYPE=Release -DZMP_HARNESS=OFF
cmake --build build\web --target soh -j 8
python web\make_web.py --build build\web\soh --out dist\web
```

`dist\web` is the browser version: serve it with `zmp-server -web dist\web` ([Browser mode](07-browser-mode.md)) or
any static web server. `make_web.py` prints the **build hash** (the first 16 hex digits of the SHA-256 of `soh.wasm`):
players with different hashes cannot share a room. To make the release zip, zip the contents of `dist\web` as
`ZMP-<version>-web.zip` and run `python tools\check_release.py` on it.

The linking step needs a lot of memory; with less than 16 GB of RAM use fewer jobs (`-j 2`).

## Server

Requirements: Go 1.26 or newer.

```powershell
cd server
go test ./...
go build -o zmp-server.exe ./cmd/zmp-server
```

All release binaries (Windows, Linux x64, Linux ARM64, static, no dependencies) at once:

```powershell
python tools\build_server.py --version 0.6.2
```

They are written to `dist\`. The Docker image builds from source: see [Host on a VPS](06-host-on-a-vps.md).

## Linux and macOS

Ship of Harkinian builds on Linux and macOS, and so may this fork, but ZMP has only been built and tested on Windows
(desktop) and in browsers. Lockstep needs every player of a room on the same build, so a Linux build could only play
with other players of that same Linux build.

## Code layout of ZMP

- `soh/soh/Zmp/`: everything of ZMP inside the game (lockstep client, per-player simulation, state transfer,
  room window, test harness).
- Changes to Ship of Harkinian's own files are marked with `// ZMP:` comments; search for them to see every place
  ZMP touches the original code.
- `soh/web/`: the browser page (`shell.html`), the ROM converter glue and ZMP's page settings (`zmp-web.js`).
- `libultraship/VENDORED.md` and `torch/VENDORED.md` list the changes made to those libraries.
- Comments often cite design decisions as `D-NNN` and test findings by letter; those refer to the project's
  development log, which is not part of this repository.
