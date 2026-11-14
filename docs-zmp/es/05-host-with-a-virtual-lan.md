# 5. Hospedar con Tailscale o ZeroTier (sin cambios en el router)

[English](../en/05-host-with-a-virtual-lan.md) | **Español**

Una LAN virtual pone tus PC en una red privada a través de internet, como si estuvieran en la misma casa. No se abre
nada en tu router, funciona detrás de CGNAT y solo las personas a las que invitas pueden llegar al servidor. Los dos
servicios de abajo tienen planes gratuitos para grupos pequeños. ZMP no depende de ninguno: cualquier VPN o LAN virtual
que permita a tus amigos llegar a tu PC por el puerto TCP 47100 funciona igual (Hamachi, WireGuard, Radmin VPN...).

En ambos casos, el anfitrión primero ejecuta `zmp-server` como en [Hospedar en tu propio PC](04-host-on-your-pc.md),
paso 1 (sáltate la parte del router).

## Tailscale

1. Todos instalan Tailscale desde <https://tailscale.com/download> e inician sesión.
2. El anfitrión invita a los amigos. Hay dos maneras:
   - **Compartir solo el PC del anfitrión** (recomendado): en la consola de administración de Tailscale, abre el menú
     de la máquina del anfitrión, elige **Share...** (compartir) y envía el enlace de invitación a cada amigo. Ven ese
     PC y nada más de tu red.
   - O invitarlos a tu tailnet como usuarios (verían todos los dispositivos que tengas en ella).
3. La dirección del anfitrión: el icono de Tailscale o `tailscale ip -4` muestra una dirección como `100.101.102.103`;
   con MagicDNS activado, también sirve un nombre como `my-pc.tailnet-name.ts.net`.
4. Los amigos escriben esa dirección en el campo **Servidor (host)** del juego, el puerto `47100` y el nombre de la
   sala.

Si un amigo no puede conectarse, comprueba que el Firewall de Windows del anfitrión permitió `zmp-server` (el tráfico
de Tailscale cuenta como red privada en la mayoría de los PC; si la red aparece como pública, permite también el
servidor en redes públicas).

## ZeroTier

1. El anfitrión crea una red en <https://my.zerotier.com> (cuenta gratuita) y copia su **Network ID** (identificador de
   red, 16 caracteres).
2. Todos instalan ZeroTier desde <https://www.zerotier.com/download/> y luego usan **Join New Network** (unirse a una
   red nueva) con ese ID.
3. El anfitrión, en la página de la red, marca **Auth** (autorizar) para cada miembro que se haya unido (en una red
   privada, que es la opción por defecto).
4. La dirección del anfitrión es la **Managed IP** (IP administrada) que aparece junto al PC del anfitrión en esa
   página (por ejemplo `10.147.17.5`).
5. Los amigos escriben esa dirección en el juego, el puerto `47100` y el nombre de la sala.

## Jugadores de navegador

Con una LAN virtual, la forma más sencilla de que los amigos jueguen en el navegador es que el propio servidor sirva la
página (`zmp-server -web <carpeta>`): abren `http://<dirección del anfitrión>:47100/` (`http` simple, que está bien
dentro de la red privada). Consulta [Modo navegador](07-browser-mode.md). Los amigos siguen necesitando Tailscale o
ZeroTier en marcha.
