# Guías de ZMP (español)

[English version](../en/README.md)

Empieza aquí si solo quieres jugar:

1. [Obtener el juego](01-get-the-game.md): descarga una versión ya hecha o compílalo tú mismo.
2. [Tu ROM](02-your-rom.md): los datos del juego salen de tu propia copia de Ocarina of Time.
3. [Jugar juntos](03-play.md): conéctate a una sala, qué se comparte y qué es solo tuyo, controles.

Alguien del grupo tiene que ejecutar el pequeño servidor de salas (uno por grupo, en cualquier PC):

4. [Hospedar en tu propio PC](04-host-on-your-pc.md) (redirección de puertos en tu router).
5. [Hospedar con Tailscale o ZeroTier](05-host-with-a-virtual-lan.md) (sin cambios en el router).
6. [Hospedar en un VPS con Docker](06-host-on-a-vps.md) (siempre encendido, HTTPS opcional para el navegador).

Y además:

7. [Modo navegador](07-browser-mode.md): juega en Chrome, Edge o Firefox, sin instalar nada.
8. [Solución de problemas](08-troubleshooting.md): cada mensaje de error y qué hacer.
9. [Compilar desde el código fuente](build.md): compilaciones de Windows, navegador y servidor, empaquetado.
10. [Publicar una versión](release.md) (para mantenedores).

## Cómo funciona ZMP, en un párrafo

ZMP es multijugador **lockstep** (en paso sincronizado): el PC de cada jugador ejecuta el juego entero, y todos reciben
exactamente las mismas entradas en el mismo orden, así que cada PC calcula exactamente el mismo mundo, fotograma a
fotograma. Lo único que hay en medio es un pequeño **servidor de retransmisión**, `zmp-server`: no ejecuta el juego,
solo ordena los botones de cada jugador, los reenvía y guarda la partida compartida de cada sala. Es un único programa
pequeño (unos pocos MB, con muy poca CPU y memoria) que uno de los jugadores puede ejecutar en su propio PC. No hay un
servidor central de ZMP, ni cuentas, ni nada en lo que registrarse: cada grupo de amigos usa su propio servidor.
