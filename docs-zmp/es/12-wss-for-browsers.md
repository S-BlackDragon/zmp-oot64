# 12. wss:// para navegadores (conexiones seguras)

[English](../en/12-wss-for-browsers.md) | **Español**

Los navegadores protegen las páginas abiertas por `https://`: una página así solo puede abrir conexiones WebSocket
**seguras**, `wss://`. Así que si tus jugadores de navegador abren el juego desde una dirección `https://`, el servidor
de salas tiene que estar disponible en una dirección `wss://`. (Una página abierta por `http://` normal, por ejemplo
directamente desde `zmp-server -web`, puede usar cualquier dirección y no necesita nada de esto.)

`zmp-server` habla TCP normal y WebSocket normal en un solo puerto. Para `wss://`, pon delante un **proxy TLS**: un
servidor web con certificado que pasa las conexiones a `zmp-server`. Necesitas un dominio que apunte a la dirección
pública del servidor.

## Con Caddy y Docker (en un VPS)

[Caddy](https://caddyserver.com/) consigue un certificado gratuito él solo. Junto al servidor de la
[guía del VPS](06-host-on-a-vps.md), añade este servicio a `server/deploy/docker-compose.yml`:

```yaml
  caddy:
    image: caddy:2
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - ./caddy-data:/data
```

y crea `server/deploy/Caddyfile` con tu dominio:

```
zmp.example.org {
    reverse_proxy zmp-server:47100
}
```

Abre los puertos 80 y 443 en el cortafuegos y luego `docker compose up -d`. Si el servidor también sirve la versión
para navegador (`-web`, [guía del VPS](06-host-on-a-vps.md), sección 6), los jugadores de navegador abren
`https://zmp.example.org/` y la página pone ella sola `wss://zmp.example.org/` como servidor.

Esta configuración de Caddy es un ejemplo estándar que el proyecto no ha probado de principio a fin; la configuración
`http` normal (`-web` en el puerto 47100) es la que cubren las pruebas del servidor.

## Quién usa qué dirección

- **Jugadores de navegador**: `wss://zmp.example.org/`.
- **Jugadores de escritorio**: pueden usar la misma dirección `wss://zmp.example.org/` (el juego de escritorio también
  habla `ws://` y `wss://`), o la dirección normal y el puerto 47100 si ese puerto está abierto.
- Un **código de invitación** puede llevar una dirección `wss://`: con
  `zmp-invite encode -addr wss://zmp.example.org/ ...` ([guía del relé](11-relay.md)), o con
  `public_address = "wss://zmp.example.org/"` en los ajustes del [servidor con doble clic](04-host-on-your-pc.md).

## Certificados

El certificado tiene que ser válido para ese nombre y de confianza para los navegadores normales (los gratuitos de
Caddy lo son). Con un certificado caducado, hecho a mano o de otro nombre, los navegadores se niegan a conectar y el
juego de escritorio dice "El certificado del servidor no es valido: no se conecta (wss)". El juego nunca cae a una
conexión insegura.

## Detrás de un proxy o un túnel

Todos los jugadores llegan a `zmp-server` a través del proxy, así que parecen venir de una sola dirección. En un
[relé](11-relay.md), apaga los límites por dirección (`-max-conns-per-ip 0 -max-rooms-per-ip 0`).
