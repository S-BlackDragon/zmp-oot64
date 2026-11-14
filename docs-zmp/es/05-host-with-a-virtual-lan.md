# 5. Plan B: Tailscale o ZeroTier (sin tocar el router)

[English](../en/05-host-with-a-virtual-lan.md) | **Español**

Una LAN virtual pone los PC del grupo en una red privada a través de internet, como si estuvieran en la misma casa. No se
abre nada en el router, funciona con CGNAT y solo pueden llegar las personas que invites. Los dos servicios de abajo
tienen planes gratuitos para grupos pequeños. ZMP no depende de ninguno: cualquier VPN o LAN virtual que permita a tus
amigos llegar a tu PC por el puerto TCP 47100 funciona igual (WireGuard, Radmin VPN, Hamachi...).

## El anfitrión

Aloja como siempre: pestaña **Alojar**, "Mi red local y quien tenga el codigo", **Alojar y jugar**
([guía principal](00-host-and-send-the-code.md)). La línea "Desde internet" habla de tu router de verdad, así que aquí
puede decir cualquier cosa: no le hagas caso. (Un PC que solo aloja puede usar en su lugar el
[servidor con doble clic](04-host-on-your-pc.md).)

Después da a tus amigos tres cosas: tu **dirección de la LAN virtual** (abajo), la **sala** y la **contraseña de la
sala** (las dos aparecen en la pestaña Alojar).

El código de invitación lleva las direcciones de tu red de casa, no siempre la virtual, así que los amigos entran
**por dirección**.

## Los amigos

Pestaña **Unirse**, sección **Por direccion**:

- **Servidor**: la dirección de la LAN virtual del anfitrión (el puerto 47100 es el de por defecto, no hace falta
  añadirlo);
- **Sala** y **Contrasena**: lo que te dio el anfitrión;
- pulsa **Conectar**.

## Tailscale

1. Todos instalan Tailscale desde <https://tailscale.com/download> e inician sesión.
2. El anfitrión invita a los amigos. Dos formas:
   - **Compartir solo el PC del anfitrión** (recomendado): en la consola de administración de Tailscale, abre el menú
     de la máquina del anfitrión, elige **Share...** y envía el enlace de invitación a cada amigo. Ven ese PC y nada
     más de tu red.
   - O invitarlos a tu tailnet como usuarios (verían todos los dispositivos que tengas en ella).
3. La dirección del anfitrión: el icono de Tailscale o `tailscale ip -4` muestra una dirección como `100.101.102.103`;
   con MagicDNS activado, también funciona un nombre como `my-pc.tailnet-name.ts.net`.

## ZeroTier

1. El anfitrión crea una red en <https://my.zerotier.com> (cuenta gratuita) y copia su **Network ID** (16
   caracteres).
2. Todos instalan ZeroTier desde <https://www.zerotier.com/download/> y luego **Join New Network** con ese ID.
3. El anfitrión, en la página de la red, marca **Auth** para cada miembro que se haya unido (en una red privada, que es
   la opción por defecto).
4. La dirección del anfitrión es la **Managed IP** que aparece junto a su PC en esa página (por ejemplo
   `10.147.17.5`).

## Si un amigo no puede conectarse

- Comprueba que el Firewall de Windows del anfitrión permitió `zmp-server`. El tráfico de la LAN virtual cuenta como
  red privada en la mayoría de los PC; si Windows muestra esa red como pública, permite el servidor también en redes
  públicas.
- Los dos PC deben aparecer como conectados en Tailscale o ZeroTier.

## Jugadores de navegador

Los jugadores de navegador pueden usar la misma LAN virtual con un servidor que sirva la página
(`zmp-server -web <carpeta>`): abren `http://<dirección del anfitrión>:47100/` (`http` normal, que está bien dentro
de la red privada). Consulta el [modo navegador](07-browser-mode.md). Siguen necesitando Tailscale o ZeroTier en
marcha.
