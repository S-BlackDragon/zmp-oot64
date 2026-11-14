# 2. Tu ROM

[English](../en/02-your-rom.md) | **Español**

ZMP, igual que Ship of Harkinian, **no incluye ninguna parte del juego original**. Los gráficos, los sonidos, la música
y el texto se construyen en tu PC a partir de tu propia copia, obtenida legalmente, de *The Legend of Zelda: Ocarina of
Time* (un volcado de ROM). Nadie puede enviarte la ROM ni los archivos hechos a partir de ella, y tú no debes compartir
la tuya.

## La regla del multijugador: todos usan la misma versión de la ROM

Todos los jugadores de una sala deben tener la **misma versión** de la ROM (por ejemplo, todos la de GameCube europea,
o todos la de N64 NTSC 1.0). Si la tuya es distinta, el juego rechaza que te unas y te dice, por nombre, qué versión
tienes tú y cuál usa la sala. Acuerden una versión antes de empezar.

La mayoría de las pruebas automáticas de ZMP usaron una versión (la ROM de depuración de GameCube PAL). Cualquier otra
versión que acepte Ship of Harkinian se carga igual, pero se ha probado menos con ZMP.

## Versiones compatibles

La misma lista que Ship of Harkinian: N64 NTSC 1.0, 1.1 y 1.2; N64 PAL 1.0 y 1.1; GameCube NTSC-U, NTSC-J, NTSC-J
Collector's Edition y PAL; Master Quest NTSC-U, NTSC-J y PAL; y las ROM de depuración de GameCube PAL y de Master
Quest. Archivos aceptados: `.z64`, `.n64` o `.v64`, sin comprimir (32, 54 o 64 MB). No un `.zip`, `.7z` o `.rar`:
extrae antes la ROM. Puedes comprobar tu volcado con el verificador de compatibilidad en <https://ship.equipment/>.

## Juego de escritorio: una sola vez

1. Abre `soh.exe`. La primera vez dice **"No O2R files found. Generate one now?"** (no se han encontrado archivos O2R,
   ¿generar uno ahora?): haz clic en **Yes** (sí) y elige tu ROM.
2. Tarda uno o dos minutos. Escribe `oot.o2r` (o `oot-mq.o2r` para Master Quest) junto a `soh.exe`; ese archivo son los
   datos del juego hechos a partir de tu ROM. Cuando pregunte **"Extract another?"** (¿extraer otra?), haz clic en
   **No**.
3. Ya está: la próxima vez el juego arranca directamente. Tu archivo de ROM no se copia ni se modifica.

No envíes tu `oot.o2r` a tus amigos: cada jugador crea el suyo a partir de su propia ROM.

## Modo navegador: en cada visita

La página te pide la ROM cada vez que la abres (botón **Choose ROM file**, elegir archivo de ROM, o suelta el archivo
sobre la página). Se lee desde tu disco y se convierte en la memoria de la pestaña del navegador en aproximadamente un
minuto; no se sube nada y no se guarda nada en el navegador. Consulta [Modo navegador](07-browser-mode.md).
