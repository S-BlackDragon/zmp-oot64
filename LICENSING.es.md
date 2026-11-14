# Licencias

[English](LICENSING.md) | **Español**

Este repositorio mezcla **nuestro propio código (ZMP)**, que está bajo la licencia MIT de [`LICENSE-ZMP`](LICENSE-ZMP), con **código de
otros proyectos**, que conserva los términos que le dieron sus autores. Nuestra licencia MIT no cambia, ni puede cambiar, esos términos.
Este repositorio es un fork de Ship of Harkinian: el juego está en la raíz, como en el original, y las carpetas propias de ZMP
están a su lado. A propósito no hay un archivo `LICENSE` en la raíz: `LICENSE-ZMP` cubre solo ZMP. Esta página dice exactamente
qué es de quién.

**No se distribuye ninguna ROM ni ningún asset del juego** en este repositorio ni en sus releases: ninguna parte de los
datos de código, gráficos, sonidos, música o textos del juego original. Cada jugador genera los datos del juego en su propia máquina
a partir de su propia copia (ver [docs-zmp/es/02-your-rom.md](docs-zmp/es/02-your-rom.md)). `tools/check_release.py` revisa cada
archivo de una release en busca de ROMs y de archivos derivados de una ROM antes de publicarla.

## Carpeta por carpeta

| Ruta | De quién | Términos |
|---|---|---|
| `server/` | ZMP | MIT ([`LICENSE-ZMP`](LICENSE-ZMP)) |
| `web/` | ZMP | MIT |
| `tools/` | ZMP | MIT |
| `docs-zmp/`, `README.md`, `README.es.md`, `LICENSING.md`, `LICENSING.es.md`, `LICENSE-ZMP`, `.github/workflows/zmp.yml` | ZMP | MIT |
| `soh/soh/Zmp/` | ZMP | MIT |
| `soh/web/zmp-web.js` | ZMP | MIT |
| Líneas y bloques marcados con `// ZMP:` (o `# ZMP`) dentro de otros archivos | Modificaciones de ZMP | Ofrecemos nuestras modificaciones bajo MIT en la medida en que se puedan separar del código que las rodea. Los archivos, en conjunto, conservan los términos del proyecto del que vienen (filas siguientes). |
| Todo lo demás de la raíz (`soh/src/`, `soh/soh/` fuera de `Zmp/`, `soh/include/`, `soh/assets/`, `CMake/`, `scripts/`, `docs/`, `README-SoH.md`, los demás flujos de `.github/`, archivos de compilación) | [Ship of Harkinian](https://github.com/HarbourMasters/Shipwright) de HarbourMasters, que se apoya en la [decompilación de Ocarina of Time](https://github.com/zeldaret/oot) de zeldaret | El repositorio de Ship of Harkinian **no tiene archivo de licencia**, así que no concede ninguna licencia propia; está publicado en GitHub, donde se puede ver y hacer fork públicamente. **No está cubierto por nuestra licencia MIT.** Este repositorio es un fork de GitHub suyo, sin cambios salvo las modificaciones de ZMP. |
| `soh/web/` (salvo `zmp-web.js`), `soh/soh/Web/`, `CMake/EmscriptenConfig.cmake` y las partes de los CMake para la compilación web | Adaptado de [ShipWeb](https://github.com/samfisher753/ShipWeb) de samfisher753, un port web no oficial de Ship of Harkinian | La misma situación que Ship of Harkinian: sin archivo de licencia propio. **No está cubierto por nuestra licencia MIT** (nuestros cambios ahí están marcados con ZMP). |
| `libultraship/` | [libultraship](https://github.com/kenix3/libultraship) de kenix3 y colaboradores | MIT: [`libultraship/LICENSE`](libultraship/LICENSE) (sin tocar). Su renderizador Fast3D: MIT, [`libultraship/src/fast/LICENSE.txt`](libultraship/src/fast/LICENSE.txt). Nuestros cambios están en [`libultraship/VENDORED.md`](libultraship/VENDORED.md). |
| `torch/` | [Torch](https://github.com/HarbourMasters/Torch) de HarbourMasters (el conversor de ROM a `o2r`) | MIT: [`torch/LICENSE`](torch/LICENSE) (sin tocar). Las bibliotecas incluidas en `torch/lib/` conservan sus propias licencias (por ejemplo StormLib, MIT: [`torch/lib/StormLib/LICENSE`](torch/lib/StormLib/LICENSE); miniz; CLI11; TinySHA1). Nuestro cambio: [`torch/VENDORED.md`](torch/VENDORED.md). |
| `soh/soh/Enhancements/randomizer/3drando/` | Derivado de [OoT-Randomizer](https://github.com/TestRunnerSRL/OoT-Randomizer) | MIT: [`LICENSE.md`](soh/soh/Enhancements/randomizer/3drando/LICENSE.md) en esa carpeta. |
| `soh/assets/custom/fonts/` | Autores de las fuentes (Noto Sans JP, Montserrat, Inconsolata, Press Start 2P, Fipps) | Sus propias licencias de fuente (SIL Open Font License o similar), tal como las distribuye Ship of Harkinian. |

Los archivos de licencia y de avisos de esos proyectos se conservan exactamente donde sus autores los pusieron, sin cambios.

## Descargado al compilar

La compilación descarga otras bibliotecas; no se guardan en este repositorio y conservan sus propias licencias:

- **Juego (vcpkg y CMake FetchContent)**: SDL2 y SDL2_net (zlib), zlib (zlib), bzip2, libzip (BSD), libpng (libpng),
  GLEW (BSD/MIT), GLFW (zlib), nlohmann-json (MIT), tinyxml2 (zlib), spdlog y fmt (MIT), libogg, libvorbis, opus y
  opusfile (BSD), zstd (BSD), Dear ImGui (MIT), y las demás que listan los archivos de CMake.
- **Servidor (módulos de Go)**: `github.com/coder/websocket` (ISC), `github.com/vmihailenco/msgpack/v5` y
  `github.com/vmihailenco/tagparser/v2` (BSD-2-Clause).

## Archivos de las releases

- `zmp-server-*`: compilado solo a partir de `server/` y sus módulos de Go: MIT más las licencias de esos módulos.
- `ZMP-*-windows.zip` y `ZMP-*-web.zip`: el juego, compilado desde las carpetas de la raíz: la mezcla descrita arriba. Incluyen
  `soh.o2r`, los datos propios de Ship of Harkinian (sus menús, fuentes e iconos, generados desde `soh/assets/custom/`), que
  no son datos de Nintendo. Nunca incluyen una ROM, un `oot.o2r`/`oot-mq.o2r` ni nada extraído de una ROM.

## Marcas

*The Legend of Zelda* y *Ocarina of Time* son marcas de Nintendo. ZMP es un proyecto de fans, **sin relación con
Nintendo, HarbourMasters (Ship of Harkinian), zeldaret ni ShipWeb, y no cuenta con su respaldo**. Por favor, no pidas a esos proyectos
ayuda con ZMP; abre una incidencia (issue) en este repositorio. No apoyamos la piratería: obtén los datos de tu propia copia del
juego.
