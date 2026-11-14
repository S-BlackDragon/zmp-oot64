# 11. Plan B: un relé

[English](../en/11-relay.md) | **Español**

Un **relé** es cualquier `zmp-server` en internet que deja crear salas a gente que no conoce. Cuando no se puede
llegar a nadie del grupo (CGNAT, sin acceso al router), todos se conectan al relé: la sala vive allí, y cada PC solo
hace conexiones de salida, que siempre funcionan.

ZMP **no trae ningún relé ni nombra ninguno**. Un relé lo pone alguien: un amigo con un VPS, una comunidad, tú mismo.
Usa uno cuyo dueño sea de fiar: el relé pasa las entradas de tu partida y guarda la partida compartida de la sala
(nunca ve tu contraseña en claro, y nunca ejecuta el juego).

## Usar un relé

Necesitas su dirección (por ejemplo `relay.example.org` o `203.0.113.10:47100`).

1. El anfitrión, en el juego: **Unirse > Por direccion**:
   - **Servidor**: la dirección del relé;
   - **Sala**: un nombre que nadie adivine (por ejemplo `bosque-k3f9`);
   - **Contrasena**: invéntate una. El primer jugador que entra en una sala con contraseña la fija;
   - **Conectar**, y luego carga la partida que será la del grupo.
2. Los amigos hacen lo mismo con el mismo servidor, sala y contraseña.

En vez de pasar tres cosas, el anfitrión puede hacer un **código de invitación** con `zmp-invite` (va en el zip de
Windows, junto a `soh.exe`; abre una terminal en esa carpeta):

```
zmp-invite encode -addr relay.example.org -room bosque-k3f9 -password inventate-una
```

Muestra el código (`ZMP-...`) y el enlace `zmp://`; los amigos lo pegan en **Unirse > Unirse con codigo**. Usa
`-port <puerto>` si el relé no está en el 47100, `-addr wss://relay.example.org/` para un relé `wss://`, y
`-hint "..."` en vez de `-password` para poner en el código solo una pista.

En un relé, la contraseña de una sala que nadie usa en 7 días se olvida; después, el nombre queda libre otra vez.

## Montar un relé

Cualquier `zmp-server` se convierte en relé con `-relay` (o `relay = true` en su archivo de ajustes, o
`ZMP_RELAY=true`):

```
zmp-server -relay
```

Escucha en todas las interfaces en el puerto 47100 y aplica límites pensados para desconocidos. Cada uno tiene su
opción ([tabla](04-host-on-your-pc.md#4-linux-una-raspberry-pi-o-sin-página)):

| Límite | Por defecto en un relé | Opción |
|---|---|---|
| Salas abiertas a la vez | 100 | `-max-rooms` |
| Conexiones de juego desde una dirección | 8 | `-max-conns-per-ip` |
| Salas creadas desde una dirección a la vez | 3 | `-max-rooms-per-ip` |
| Olvidar la contraseña de una sala sin uso tras | 7 días | `-room-key-ttl` |
| Salas sin contraseña | permitidas | `-require-room-password` para rechazarlas |
| Lista de salas | nunca se publica | |

- Detrás de un proxy o túnel que oculta las direcciones de los jugadores, todos parecen venir de la misma dirección:
  apaga los límites por dirección (`-max-conns-per-ip 0 -max-rooms-per-ip 0`).
- En un VPS, ejecútalo con Docker ([guía del VPS](06-host-on-a-vps.md)) y añade `ZMP_RELAY: "true"` a su
  `environment:`.
- Para jugadores de navegador detrás de `https`, pon un proxy TLS delante ([wss:// para navegadores](12-wss-for-browsers.md)).
- Un sitio que sirva la versión para navegador puede apuntarla a su relé con `zmp-config.json`
  ([guía](10-web-version-on-your-site.md)).
