# Licensing

**English** | [Español](LICENSING.es.md)

This repository mixes **our own code (ZMP)**, which is under the MIT license in [`LICENSE-ZMP`](LICENSE-ZMP), with **code from
other projects**, which keeps the terms its authors gave it. Our MIT license does not and cannot change those terms.
This repository is a fork of Ship of Harkinian: the game is at the root, as in the original, and ZMP's own folders
sit next to it. There is deliberately no root `LICENSE` file: `LICENSE-ZMP` covers only ZMP. This page says exactly
which is which.

**No ROM and no game asset is distributed** in this repository or in its releases: no part of the original game's
code data, graphics, sounds, music or text. Every player builds the game data on their own machine from their own
copy of the game (see [docs-zmp/en/02-your-rom.md](docs-zmp/en/02-your-rom.md)). `tools/check_release.py` verifies every
release artifact for ROMs and ROM-derived archives before it is published.

## Folder by folder

| Path | Whose | Terms |
|---|---|---|
| `server/` | ZMP | MIT ([`LICENSE-ZMP`](LICENSE-ZMP)) |
| `web/` | ZMP | MIT |
| `tools/` | ZMP | MIT |
| `docs-zmp/`, `README.md`, `README.es.md`, `LICENSING.md`, `LICENSING.es.md`, `LICENSE-ZMP`, `.github/workflows/zmp.yml` | ZMP | MIT |
| `soh/soh/Zmp/` | ZMP | MIT |
| `soh/web/zmp-web.js` | ZMP | MIT |
| Lines and blocks marked `// ZMP:` (or `# ZMP`) inside other files | ZMP's modifications | Our modifications are offered under MIT as far as they can be separated from the code around them. The files as a whole keep the terms of the project they come from (rows below). |
| Everything else at the root (`soh/src/`, `soh/soh/` outside `Zmp/`, `soh/include/`, `soh/assets/`, `CMake/`, `scripts/`, `docs/`, `README-SoH.md`, the other `.github/` workflows, build files) | [Ship of Harkinian](https://github.com/HarbourMasters/Shipwright) by HarbourMasters, which builds on the [Ocarina of Time decompilation](https://github.com/zeldaret/oot) by zeldaret | Ship of Harkinian's repository has **no license file**, so it grants no license of its own; it is published on GitHub, where it is publicly viewable and forkable. **Not covered by our MIT license.** This repository is a GitHub fork of it, unchanged except for the ZMP modifications. |
| `soh/web/` (except `zmp-web.js`), `soh/soh/Web/`, `CMake/EmscriptenConfig.cmake` and the browser-build parts of the CMake files | Adapted from [ShipWeb](https://github.com/samfisher753/ShipWeb) by samfisher753, an unofficial web port of Ship of Harkinian | Same situation as Ship of Harkinian: no license file of its own. **Not covered by our MIT license** (our changes in them are marked ZMP). |
| `libultraship/` | [libultraship](https://github.com/kenix3/libultraship) by kenix3 and contributors | MIT: [`libultraship/LICENSE`](libultraship/LICENSE) (unchanged). Its Fast3D renderer: MIT, [`libultraship/src/fast/LICENSE.txt`](libultraship/src/fast/LICENSE.txt). Our changes are listed in [`libultraship/VENDORED.md`](libultraship/VENDORED.md). |
| `torch/` | [Torch](https://github.com/HarbourMasters/Torch) by HarbourMasters (the ROM-to-`o2r` converter) | MIT: [`torch/LICENSE`](torch/LICENSE) (unchanged). Bundled libraries in `torch/lib/` keep their own licenses (for example StormLib, MIT: [`torch/lib/StormLib/LICENSE`](torch/lib/StormLib/LICENSE); miniz; CLI11; TinySHA1). Our change: [`torch/VENDORED.md`](torch/VENDORED.md). |
| `soh/soh/Enhancements/randomizer/3drando/` | Derived from [OoT-Randomizer](https://github.com/TestRunnerSRL/OoT-Randomizer) | MIT: [`LICENSE.md`](soh/soh/Enhancements/randomizer/3drando/LICENSE.md) in that folder. |
| `soh/assets/custom/fonts/` | Font authors (Noto Sans JP, Montserrat, Inconsolata, Press Start 2P, Fipps) | Their own font licenses (SIL Open Font License or similar), as distributed by Ship of Harkinian. |

The license and notice files of those projects are kept exactly where their authors put them, unchanged.

## Downloaded when you build

The build downloads other libraries; they are not stored in this repository and keep their own licenses:

- **Game (vcpkg and CMake FetchContent)**: SDL2 and SDL2_net (zlib), zlib (zlib), bzip2, libzip (BSD), libpng (libpng),
  GLEW (BSD/MIT), GLFW (zlib), nlohmann-json (MIT), tinyxml2 (zlib), spdlog and fmt (MIT), libogg, libvorbis, opus and
  opusfile (BSD), zstd (BSD), Dear ImGui (MIT), and the others listed by the CMake files.
- **Server (Go modules)**: `github.com/coder/websocket` (ISC), `github.com/vmihailenco/msgpack/v5` and
  `github.com/vmihailenco/tagparser/v2` (BSD-2-Clause).

## Release artifacts

- `zmp-server-*`: built only from `server/` and its Go modules: MIT plus the Go modules' licenses.
- `ZMP-*-windows.zip` and `ZMP-*-web.zip`: the game, built from the root folders: the mix described above. They include
  `soh.o2r`, Ship of Harkinian's own data (its menus, fonts and icons, built from `soh/assets/custom/`), which
  is not Nintendo data. They never include a ROM, an `oot.o2r`/`oot-mq.o2r`, or anything extracted from a ROM.

## Trademarks

*The Legend of Zelda* and *Ocarina of Time* are trademarks of Nintendo. ZMP is a fan project, **not affiliated with
or endorsed by Nintendo, HarbourMasters (Ship of Harkinian), zeldaret or ShipWeb**. Please do not ask those projects
for help with ZMP; open an issue in this repository instead. We do not condone piracy: dump your own copy of the
game.
