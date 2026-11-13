# Compilar desde el código fuente

Todo lo necesario está en este repositorio:

| Carpeta | Qué contiene | Herramienta de compilación |
|---|---|---|
| raíz (`soh/`, `libultraship/`, `torch/`, `CMakeLists.txt`...) | el juego: Ship of Harkinian con ZMP (`soh/soh/Zmp/`), libultraship y torch como carpetas normales | CMake + Visual Studio (Windows), Emscripten (navegador) |
| `server/` | `zmp-server`, el servidor de salas | Go |
| `web/` | `make_web.py`: convierte la compilación del navegador en una carpeta que se puede servir | Python 3 |
| `tools/` | `package_windows.py` (zip para jugadores), `build_server.py` (binarios del servidor), `check_release.py` (sin datos de ROM en una versión publicada) | Python 3 |

No hace falta ninguna ROM ni datos del juego para compilar; solo se necesitan para jugar.

Los comandos de abajo se ejecutan en **PowerShell** desde la raíz del repositorio, salvo que se indique otra cosa.

## Juego para Windows

Requisitos (versiones que se sabe que funcionan):

- Visual Studio 2022 (Community sirve) con la carga de trabajo **Desarrollo para el escritorio con C++** (MSVC v143,
  SDK de Windows);
- CMake 3.26 o más reciente, Git, Python 3.

CMake descarga y compila las bibliotecas que necesita el juego con **vcpkg** la primera vez (clona vcpkg en
`$env:VCPKG_ROOT`, o en la carpeta de compilación si esa variable no está definida). La primera configuración tarda un
buen rato; las siguientes son rápidas. También descarga algunas bibliotecas fijadas (Dear ImGui y otras) con
FetchContent.

```powershell
$env:VCPKG_ROOT = "$PWD\vcpkg"     # optional: one vcpkg for all your builds
cmake -S . -B build\win -G "Visual Studio 17 2022" -T v143 -A x64 -DCMAKE_BUILD_TYPE:STRING=Release -DZMP_HARNESS=OFF
cmake --build build\win --target GenerateSohOtr --config Release
cmake --build build\win --config Release --parallel
```

- `-DZMP_HARNESS=OFF` es la compilación para jugadores. `ON` (el valor por defecto) añade el arnés de pruebas que usa el
  proyecto para sus pruebas automáticas (un puerto de control por instancia del juego); los jugadores no lo necesitan.
- El juego queda en `x64\Release\soh.exe`, y los datos propios de Ship of Harkinian, `soh.o2r`, en
  `build\win\soh\soh.o2r` (también se escribe una copia en `soh.o2r`, en la raíz del repositorio; la compilación del navegador la necesita).
- Compilación completa: unos 8 minutos en un PC de 12 núcleos, con vcpkg ya listo.

Empaquétalo como una versión publicada (no puede acabar ningún dato de ROM en él: el script lo comprueba):

```powershell
cd build\win; cpack -G ZIP -C Release; cd ..\..
python tools\package_windows.py --build (Get-ChildItem _packages\*.zip | Select-Object -First 1).FullName --version 0.6.0
```

El resultado es `dist\ZMP-0.6.0-windows.zip`. Todos los jugadores de una sala necesitan el mismo `soh.exe`, así que da
a tus amigos este zip en lugar de dejar que cada uno compile el suyo.

## Juego para navegador

Requisitos: el [SDK de Emscripten](https://emscripten.org/docs/getting_started/downloads.html) (probado con 7.0.0),
Ninja, Python 3 y el `soh.o2r` de la compilación de Windows (ejecuta al menos antes el paso `GenerateSohOtr` de arriba:
la compilación del navegador empaqueta el `soh.o2r` de la raíz).

```powershell
& <emsdk folder>\emsdk_env.ps1
emcmake cmake -S . -B build\web -G Ninja -DCMAKE_BUILD_TYPE=Release -DZMP_HARNESS=OFF
cmake --build build\web --target soh -j 8
python web\make_web.py --build build\web\soh --out dist\web
```

`dist\web` es la versión para navegador: sírvela con `zmp-server -web dist\web` ([Modo navegador](07-browser-mode.md)) o
con cualquier servidor web estático. `make_web.py` muestra el **hash de compilación** (los primeros 16 dígitos
hexadecimales del SHA-256 de `soh.wasm`): los jugadores con hashes distintos no pueden compartir sala. Para hacer el zip
de la versión publicada, comprime el contenido de `dist\web` como `ZMP-<version>-web.zip` y ejecuta
`python tools\check_release.py` sobre él.

El paso de enlazado necesita mucha memoria; con menos de 16 GB de RAM usa menos trabajos (`-j 2`).

## Servidor

Requisitos: Go 1.26 o más reciente.

```powershell
cd server
go test ./...
go build -o zmp-server.exe ./cmd/zmp-server
```

Todos los binarios de la versión publicada (Windows, Linux x64, Linux ARM64, estáticos, sin dependencias) de una vez:

```powershell
python tools\build_server.py --version 0.6.0
```

Se escriben en `dist\`. La imagen de Docker se compila desde el código fuente: consulta [Hospedar en un VPS](06-host-on-a-vps.md).

## Linux y macOS

Ship of Harkinian se compila en Linux y macOS, y puede que este fork también, pero ZMP solo se ha compilado y probado
en Windows (escritorio) y en navegadores. Lockstep necesita que todos los jugadores de una sala tengan la misma
compilación, así que una compilación para Linux solo podría jugar con otros jugadores de esa misma compilación para
Linux.

## Estructura del código de ZMP

- `soh/soh/Zmp/`: todo lo de ZMP dentro del juego (cliente lockstep, simulación por jugador, transferencia de
  estado, ventana de la sala, arnés de pruebas).
- Los cambios en los archivos propios de Ship of Harkinian están marcados con comentarios `// ZMP:`; búscalos para ver
  todos los lugares donde ZMP toca el código original.
- `soh/web/`: la página del navegador (`shell.html`), el código de enlace del conversor de ROM y los ajustes de la
  página de ZMP (`zmp-web.js`).
- `libultraship/VENDORED.md` y `torch/VENDORED.md` enumeran los cambios hechos en esas bibliotecas.
- Los comentarios citan a menudo decisiones de diseño como `D-NNN` y hallazgos de pruebas por letra; se refieren al
  registro de desarrollo del proyecto, que no forma parte de este repositorio.
