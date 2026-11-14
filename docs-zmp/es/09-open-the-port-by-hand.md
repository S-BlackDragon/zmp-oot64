# 9. Plan B: abrir el puerto a mano en el router

[English](../en/09-open-the-port-by-hand.md) | **Español**

Cuando alojas, el juego pide al router que abra el puerto él solo. Si la línea "Desde internet" dice **"Router sin
UPnP"** o **"El router respondió pero no abrió el puerto"**, puedes abrirlo tú en la configuración del router. Son
cinco minutos, una sola vez.

Esto **no** sirve con **"Posible CGNAT"**: en ese caso desde internet no se llega a tu router de ninguna forma. Usa
[Tailscale o ZeroTier](05-host-with-a-virtual-lan.md), un [relé](11-relay.md) o un [VPS](06-host-on-a-vps.md).

## 1. La dirección local de tu PC

Aloja una vez desde el juego: debajo de la línea "Desde internet", la pestaña Alojar muestra "Desde un navegador de tu
red: http://**192.168.1.20**:47100/". Ese número es la dirección de tu PC en tu red de casa. (O abre una terminal,
ejecuta `ipconfig` y busca la "Dirección IPv4" de tu adaptador de red.)

## 2. Haz que esa dirección sea fija

En el router, busca la opción llamada "reserva DHCP", "concesión estática" o "reserva de dirección", y reserva esa
dirección para tu PC. Si no, un día puede cambiar y la regla de abajo deja de funcionar.

## 3. Añade la regla de reenvío

1. Abre la configuración del router en un navegador (suele estar en `http://192.168.1.1` o parecida; la
   dirección y la contraseña suelen venir en una pegatina del router).
2. Busca **Reenvío de puertos** (también llamado "Port forwarding", "Servidor virtual", "NAT" o "Asignación de
   puertos").
3. Añade una regla: protocolo **TCP**, puerto externo **47100**, puerto interno **47100**, hacia la dirección local de
   tu PC.
4. Guarda.

Si alojas en otro puerto en la pestaña Alojar, reenvía ese.

## 4. Diles a tus amigos adónde conectarse

El router no le cuenta al juego una regla que hiciste a mano, así que el código de invitación de la pestaña Alojar
solo lleva las direcciones de tu red de casa. Los amigos de internet entran **por dirección**:

- **Servidor**: tu **dirección pública** (busca "cuál es mi IP" en un navegador, o mírala en la página de estado del
  router);
- **Sala** y **Contrasena**: aparecen en tu pestaña Alojar;
- pulsan **Conectar** en **Unirse > Por direccion**.

Con el [servidor con doble clic](04-host-on-your-pc.md) puedes poner tu dirección pública (o un dominio) en
`public_address` de `zmp-server.toml`, y `port_mapping = false`: así el código la lleva, y los amigos solo tienen que
pegar el código.

Cuando un amigo entra desde internet, la línea "Desde internet" dice "Se llega desde internet".

## Si sigue sin funcionar

- Si la dirección "WAN" o "Internet" del router es distinta de la dirección pública que muestra el navegador, o empieza
  por `100.64` a `100.127`, tu operadora usa **CGNAT** y abrir puertos no puede funcionar: usa un plan B de la lista
  de arriba.
- Comprueba que el Firewall de Windows permitió `zmp-server` (también en redes públicas, si tu red está marcada como
  pública).
- Prueba desde fuera: abre `http://<tu dirección pública>:47100/healthz` desde un móvil con datos (no con tu Wi-Fi).
  Debería responder `zmp-server ... protocol 5`. Probar desde tu propio PC con tu dirección pública puede no funcionar
  en algunos routers; eso no significa que esté roto para los demás.

## Seguridad

- Solo se abre el puerto 47100, y solo escucha en él `zmp-server`. Nunca ejecuta el juego, nunca recibe archivos de
  los jugadores aparte del estado del juego y nunca guarda nada fuera de su carpeta de datos.
- La sala tiene contraseña, y tras unos cuantos intentos fallidos una dirección tiene que esperar.
- Cuando ya no vayas a alojar, quita la regla de reenvío.
