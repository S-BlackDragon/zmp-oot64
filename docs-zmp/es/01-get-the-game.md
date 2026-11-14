# 1. Obtener el juego

[English](../en/01-get-the-game.md) | **Español**

Necesitas **Windows 10 u 11, de 64 bits** para el juego de escritorio, o un navegador de escritorio reciente (Chrome,
Edge, Firefox) para el [modo navegador](07-browser-mode.md). Se recomienda un mando; el teclado también funciona.

**Todos en una sala deben usar exactamente la misma compilación**: el mismo `soh.exe` (escritorio) o el mismo
`soh.wasm` (navegador). El juego lo comprueba al unirte y rechaza una compilación distinta. Los jugadores de escritorio
y los de navegador no pueden compartir sala (son compilaciones diferentes), pero un mismo servidor puede alojar salas
de escritorio y salas de navegador a la vez.

## Opción A: descargar una versión (lo más fácil)

1. Abre la [última versión](https://github.com/S-BlackDragon/zmp-oot64/releases/latest) del repositorio en GitHub y descarga:
   - `ZMP-<version>-windows.zip`: el juego de escritorio. Ya incluye el servidor de salas (`zmp-server.exe`, el que usa
     el botón **Alojar y jugar**) y `zmp-invite.exe` (hace códigos de invitación a mano); es todo lo que necesita un
     grupo;
   - `ZMP-<version>-web.zip`: la versión para navegador, solo si quieres el [modo navegador](07-browser-mode.md);
   - `zmp-server-<version>-...` y `zmp-invite-<version>-...` (Windows, Linux x64, Linux ARM64): el servidor y la
     herramienta de códigos por separado, solo para un [PC que solo aloja](04-host-on-your-pc.md), un
     [VPS](06-host-on-a-vps.md) o un [relé](11-relay.md);
   - los archivos `.sha256.txt`: sumas de comprobación, para verificar una descarga si quieres.
2. Descomprime `ZMP-<version>-windows.zip` en una carpeta nueva y normal (por ejemplo `C:\Games\ZMP`). No lo ejecutes
   desde dentro del zip y evita `Program Files` (el juego escribe sus archivos junto a `soh.exe`).
3. Abre `soh.exe`. Si Windows muestra "Windows protegió su PC", haz clic en **Más información** y luego en **Ejecutar
   de todas formas** (el programa no está firmado).
4. La primera vez te pedirá tu ROM: consulta [Tu ROM](02-your-rom.md).
5. Después, [pulsa Alojar partida y pásale el código a tu amigo](00-host-and-send-the-code.md).

La versión no contiene ninguna parte del juego original: ni ROM, ni gráficos, ni sonidos, ni texto suyo.

## Opción B: compilarlo tú mismo

Todo está en este repositorio: el juego (en la raíz: nuestro fork de Ship of Harkinian con ZMP dentro), el servidor
(`server/`) y las herramientas. Sigue [Compilar desde el código fuente](build.md). Si lo compilas tú, los amigos con
los que juegues necesitan **tu** compilación (envíales tu zip), porque una compilación hecha por ti no es idéntica,
byte a byte, a la de la versión publicada.

## Actualizar

Cuando sale una versión nueva, todo el grupo actualiza a la vez (el juego rechaza compilaciones que no coinciden). Para
conservar tus ajustes y partidas, copia los archivos nuevos sobre la carpeta antigua; o empieza en una carpeta nueva y
copia `Save\`, `shipofharkinian.json` y, si alojas desde el juego, `zmp-host\`. El progreso compartido de una sala
vive en el servidor (para el botón **Alojar y jugar**, en esa carpeta `zmp-host`), así que no se pierde al actualizar el
juego, siempre que la versión nueva siga pudiendo abrirlo (las notas de la versión dirán si no puede).
