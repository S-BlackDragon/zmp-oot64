# 8. Solución de problemas

Los mensajes propios de ZMP dentro del juego están en español; aquí se explica cada uno.

## Al unirse a una sala

| Mensaje | Significado | Qué hacer |
|---|---|---|
| "No se pudo conectar a ..." | No se pudo establecer la conexión con el servidor | Comprueba la dirección y el puerto; comprueba que la ventana del servidor del anfitrión está abierta; para jugar por internet, que el puerto está redirigido ([guía](04-host-on-your-pc.md)) o que todos están en la misma [LAN virtual](05-host-with-a-virtual-lan.md). |
| "Rechazado por ... build" (build_mismatch) | La compilación de tu juego es distinta de la de la sala | Todos deben usar exactamente la misma versión (el mismo `soh.exe` o, en el navegador, la misma página). Descarga el mismo zip. Los jugadores de escritorio y los de navegador no pueden compartir sala. |
| "Tu ROM es X y la sala usa Y" (oot_mismatch) | La versión de tu ROM es distinta de la de la sala | Todos usan la misma versión de la ROM ([Tu ROM](02-your-rom.md)). |
| "Rechazado por ... cvar" | Tus ajustes de juego son distintos de los de la sala | Vuelve a poner las opciones "Enhancements" / "Cheats" en sus valores por defecto. La forma más rápida: cierra el juego, borra `shipofharkinian.json` en la carpeta del juego y ábrelo de nuevo. |
| "Rechazado por ... name" (name_taken) | Alguien de la sala ya usa ese nombre | Elige otro nombre. (Si eras tú tras un cierre inesperado, espera unos segundos y conéctate de nuevo con el mismo nombre para recuperar tu Link.) |
| "El nombre debe tener entre 1 y 32 caracteres" | El nombre debe tener de 1 a 32 caracteres | Escribe un nombre más corto o que no esté vacío. |
| "Version de protocolo ..." (protocol_mismatch) | Las versiones del juego y del servidor no coinciden | Actualiza el servidor y el juego a la misma versión. |
| "Esperando a ..." | Se espera a un jugador | Solo aparece cuando la conexión de alguien se detiene de verdad durante más de un segundo. Desaparece sola, o ese jugador se cae. |
| "DESYNC: resincronizado" | Los PC no coincidían y se corrigieron | Nada; se arregla solo. Si ocurre a menudo, envía los registros (abajo). |

## Modo navegador

| Problema | Qué hacer |
|---|---|
| La página se queda en "Downloading..." o "Preparing..." (descargando / preparando) | Espera (la primera carga descarga unas decenas de MB); después recarga. Revisa la consola del navegador (F12, Console). |
| "This file is compressed..." / "...isn't a supported version..." / "...doesn't match a known good one" (este archivo está comprimido / no es una versión compatible / no coincide con uno válido conocido) | Elige la ROM `.z64`/`.n64`/`.v64` sin comprimir; un volcado modificado o defectuoso se rechaza. |
| Aviso "This page was opened over https..." (esta página se abrió por https) | Usa una dirección `wss://`, o abre la página por `http` (por ejemplo desde el `zmp-server -web` del anfitrión). |
| "Conectando..." nunca pasa a "Conectado" | La dirección o el puerto son incorrectos o están bloqueados; el navegador bloquea una página `https` que intenta una dirección `ws://`. |
| "The game stopped unexpectedly" (el juego se detuvo inesperadamente) / la pestaña se cae | Normalmente es memoria: cada pestaña necesita aproximadamente 1 GB. Cierra otras pestañas y programas. |
| Al abrir `index.html` con doble clic no se ve nada | La página debe ser servida por un servidor web (`zmp-server -web`, o `python -m http.server`). |

## Servidor

- **La ventana se cierra de inmediato**: ejecútalo desde una terminal para leer el error. "address already in use"
  (dirección ya en uso) significa que otro programa (u otra copia del servidor) usa el puerto 47100: ciérralo, o usa
  `-listen :47101` (y comunica a tus amigos el nuevo puerto).
- **Los amigos no pueden llegar a él pero en tu PC funciona**: es la redirección del router o el cortafuegos. Abre
  `http://<tu dirección pública>:47100/healthz` desde un teléfono con datos móviles (no tu Wi-Fi): debe responder
  `zmp-server ... protocol 4`. Si no responde, el puerto no es accesible desde internet.
- **Empezar una sala de nuevo**: usa un nombre de sala nuevo, o haz que el creador de la sala marque "Empezar la sala
  con mi partida" antes de conectarse.

## Enviar un informe de error

Abre una incidencia (issue) en GitHub con:

- qué pasó, a qué hora y qué estabas haciendo;
- una captura de pantalla;
- la carpeta `logs` de la carpeta del juego de cada jugador (`logs\zmp.log` es el registro propio de ZMP);
- para los jugadores de navegador, el texto de la consola (F12, Console, clic derecho, "Save as...");
- el registro del servidor (su ventana, o `docker compose logs`).

Los registros contienen nombres de jugadores, el nombre de la sala y la dirección del servidor, y rutas de archivos que
pueden incluir tu nombre de usuario de Windows: léelos antes de publicarlos.
