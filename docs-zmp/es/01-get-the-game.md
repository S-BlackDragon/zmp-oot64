# 1. Obtener el juego

Necesitas **Windows 10 u 11, de 64 bits** para el juego de escritorio, o un navegador de escritorio reciente (Chrome,
Edge, Firefox) para el [modo navegador](07-browser-mode.md). Se recomienda un mando; el teclado también funciona.

**Todos en una sala deben usar exactamente la misma compilación**: el mismo `soh.exe` (escritorio) o el mismo
`soh.wasm` (navegador). El juego lo comprueba al unirte y rechaza una compilación distinta. Los jugadores de escritorio
y los de navegador no pueden compartir sala (son compilaciones diferentes), pero un mismo servidor puede alojar salas
de escritorio y salas de navegador a la vez.

## Opción A: descargar una versión (lo más fácil)

1. Abre la [última versión](https://github.com/S-BlackDragon/zmp-oot64/releases/latest) del repositorio en GitHub y descarga, de la misma versión:
   - `ZMP-<version>-windows.zip`: el juego de escritorio;
   - `zmp-server-<version>-windows-amd64.exe` (o la compilación para Linux): el servidor de salas, solo para quien
     hospede;
   - `ZMP-<version>-web.zip`: la versión para navegador, solo si quieres el [modo navegador](07-browser-mode.md).
2. Descomprime `ZMP-<version>-windows.zip` en una carpeta nueva y normal (por ejemplo `C:\Games\ZMP`). No lo ejecutes
   desde dentro del zip y evita `Program Files` (el juego escribe sus archivos junto a `soh.exe`).
3. Abre `soh.exe`. Si Windows muestra "Windows protegió su PC", haz clic en **Más información** y luego en **Ejecutar
   de todas formas** (el programa no está firmado).
4. La primera vez te pedirá tu ROM: consulta [Tu ROM](02-your-rom.md).

La versión no contiene ninguna parte del juego original: ni ROM, ni gráficos, ni sonidos, ni texto suyo.

## Opción B: compilarlo tú mismo

Todo está en este repositorio: el juego (en la raíz: nuestro fork de Ship of Harkinian con ZMP dentro), el servidor
(`server/`) y las herramientas. Sigue [Compilar desde el código fuente](build.md). Si lo compilas tú, los amigos con
los que juegues necesitan **tu** compilación (envíales tu zip), porque una compilación hecha por ti no es idéntica,
byte a byte, a la de la versión publicada.

## Actualizar

Cuando sale una versión nueva, todo el grupo actualiza a la vez (el juego rechaza compilaciones que no coinciden). Para
conservar tus ajustes y partidas, copia los archivos nuevos sobre la carpeta antigua; o empieza en una carpeta nueva y
copia `Save\` y `shipofharkinian.json`. El progreso compartido de una sala vive en el servidor, así que no se pierde al
actualizar el juego, siempre que la versión nueva siga pudiendo abrirlo (las notas de la versión dirán si no puede).
