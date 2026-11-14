# Publicar una versión (para mantenedores)

[English](../en/release.md) | **Español**

Una versión (Release) de ZMP en GitHub tiene estos archivos, todos compilados a partir del mismo commit de este
repositorio:

| Archivo | Qué es | Lo genera |
|---|---|---|
| `ZMP-<version>-windows.zip` | el juego de escritorio (soh.exe, soh.o2r, las descripciones del conversor de ROM, `zmp-server.exe` para el botón Alojar, `zmp-invite.exe`, PLAYING.txt / JUGAR.txt, licencias) | `tools/package_windows.py` con `--server` |
| `ZMP-<version>-web.zip` | la versión para navegador (index.html, soh.js, soh.wasm, soh.data, el conversor de ROM en el navegador, favicon, iconos, `app.webmanifest`, licencias; sin `zmp-config.json`) | `web/make_web.py`, y luego comprimir la carpeta |
| `zmp-server-<version>-windows-amd64.exe`, `-linux-amd64`, `-linux-arm64` | el servidor de salas (el de Windows lleva el icono de ZMP) | `tools/build_server.py` |
| `zmp-invite-<version>-...` (los mismos tres sistemas) | la herramienta de códigos de invitación | `tools/build_server.py` |
| `*.sha256.txt` | sumas de comprobación (y el hash de soh.exe que deben compartir todos los jugadores de una sala) | los mismos scripts |

**Nunca** adjuntes una ROM, un `oot.o2r` / `oot-mq.o2r`, ni nada extraído de una ROM. Cada script ejecuta
`tools/check_release.py` sobre lo que genera; vuelve a ejecutarlo sobre cada archivo justo antes de subirlo:

```powershell
Get-ChildItem dist\*.zip | ForEach-Object { python tools\check_release.py $_.FullName }
```

## Dónde compilar

Compila la versión publicada en la CI (`.github/workflows/ci.yml` sube el zip de Windows como artefacto) o en un PC con
un **vcpkg nuevo y sin caché binaria** (`$env:VCPKG_BINARY_SOURCES = "clear"`, `VCPKG_ROOT` fuera de tu carpeta de
usuario). Las bibliotecas que salen de una caché de vcpkg guardan dentro de `soh.exe` las rutas absolutas de la
máquina que las compiló, que pueden incluir un nombre de usuario de Windows; `check_release.py` rechaza ese archivo.

## Pasos

1. Asegúrate de que `go test ./...` pasa en `server/`.
2. Si cambió el icono, vuelve a generar el recurso de icono de Windows del servidor (está en el repositorio, así que
   casi nunca hace falta):
   `python tools\make_icon_syso.py branding-zmp\icono-zmp.ico server\cmd\zmp-server\rsrc_windows_amd64.syso`.
   `go build` lo mete solo en `zmp-server.exe`.
3. `python tools\build_server.py --version <version>`: las pruebas, y después `zmp-server` y `zmp-invite` para cada
   sistema, en `dist\`.
4. Compila el juego de Windows con `-DZMP_HARNESS=OFF` ([build.md](build.md)) y empaquétalo con el servidor y la
   herramienta de códigos dentro: `python tools\package_windows.py --build <cpack zip> --version <version> --server ...`
   (`--help` muestra los argumentos exactos). Comprueba que el zip tiene `zmp-server.exe` y `zmp-invite.exe` junto a
   `soh.exe`: sin ellos, el botón Alojar dice que falta el servidor.
5. Compila el juego para navegador ([build.md](build.md)), luego
   `python web\make_web.py --build build\web\soh --out dist\web` (añade el favicon, los iconos y `app.webmanifest`) y
   comprime el **contenido** de `dist\web` como `dist\ZMP-<version>-web.zip`. Anota en las notas de la versión el hash
   de compilación que se muestra.
6. Ejecuta `tools\check_release.py` sobre cada zip (arriba). Cada uno debe decir `clean`.
7. Prueba rápida antes de publicar, todo en `127.0.0.1`: dos instancias de escritorio desde el zip (con tu propia ROM),
   una pulsa **Alojar y jugar** con "Solo este PC" y la otra entra con el código; dos pestañas de navegador servidas por
   `zmp-server -web dist\web` se unen a una sala.
8. Etiqueta el commit (`git tag v<version>`), sube la etiqueta, crea la Release en GitHub a partir de la etiqueta,
   adjunta los archivos y escribe las notas: qué cambió, los hashes de compilación, si las salas guardadas por la
   versión anterior siguen abriéndose y que todos los jugadores deben actualizar.

## Reglas de versión

- Una compilación nueva del juego (cualquier cambio en el código del juego) significa una versión nueva para todos: las
  salas rechazan compilaciones que no coinciden.
- El servidor es compatible entre versiones del juego mientras el número de protocolo siga igual (`zmp-server -version`
  lo muestra); un cambio de protocolo se indica en las notas. Un servidor de protocolo 5 (0.7.0) sigue dejando entrar a
  juegos de protocolo 4 (0.6.x) en salas sin contraseña.
