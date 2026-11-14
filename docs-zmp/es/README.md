# Guías de ZMP (español)

[English](../en/README.md) | **Español**

## Empieza aquí

**[Pulsa Alojar partida y pásale el código a tu amigo](00-host-and-send-the-code.md).** Uno del grupo pulsa **Alojar
y jugar** en el juego y pasa el código de invitación; los demás lo pegan y a jugar. Es todo lo que necesitan casi
todos los grupos.

Antes de eso, una sola vez:

1. [Obtener el juego](01-get-the-game.md): descarga una versión ya hecha o compílalo tú mismo.
2. [Tu ROM](02-your-rom.md): los datos del juego salen de tu propia copia de Ocarina of Time.
3. [Jugar juntos](03-play.md): todas las formas de unirse, qué se comparte y qué es solo tuyo, guardar, controles.

## Plan B: cuando los amigos de internet no llegan al anfitrión

- [Un PC que solo aloja](04-host-on-your-pc.md): `zmp-server.exe` con doble clic, con su propia página, su archivo de
  ajustes y sus opciones.
- [Abrir el puerto a mano en el router](09-open-the-port-by-hand.md) (cuando el router no tiene UPnP).
- [Tailscale o ZeroTier](05-host-with-a-virtual-lan.md): una red privada por internet, sin tocar el router, funciona
  con CGNAT.
- [Un relé](11-relay.md): un `zmp-server` en internet donde vive la sala; cómo usar uno y cómo montarlo.
- [Un VPS con Docker](06-host-on-a-vps.md): siempre encendido.
- [wss:// para navegadores](12-wss-for-browsers.md): conexiones seguras para páginas servidas por `https`.

## Y además

- [Modo navegador](07-browser-mode.md): juega en Chrome, Edge o Firefox, sin instalar nada.
- [Pon la versión web en tu sitio](10-web-version-on-your-site.md): `zmp-config.json` para dejar puestos tu servidor,
  la sala y las contraseñas ([esquema](../zmp-config.schema.json)).
- [Solución de problemas](08-troubleshooting.md): cada mensaje y qué hacer.
- [Compilar desde el código fuente](build.md): compilaciones de Windows, navegador y servidor, empaquetado.
- [Publicar una versión](release.md) (para mantenedores).

## Cómo funciona ZMP, en un párrafo

ZMP es multijugador **lockstep** (en paso sincronizado): el PC de cada jugador ejecuta el juego entero, y todos reciben
exactamente las mismas entradas en el mismo orden, así que cada PC calcula exactamente el mismo mundo, fotograma a
fotograma. Lo único que hay en medio es un pequeño **servidor de retransmisión**, `zmp-server`: no ejecuta el juego,
solo ordena los botones de cada jugador, los reenvía y guarda la partida compartida de cada sala. El juego lo arranca
por ti cuando pulsas **Alojar y jugar**; también puede funcionar solo en cualquier PC, en un VPS o como relé. No hay
un servidor central de ZMP, ni cuentas, ni nada en lo que registrarse: cada grupo de amigos usa su propio servidor.
