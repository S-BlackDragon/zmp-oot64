# 4. Hospedar en tu propio PC (redirección de puertos)

Una persona del grupo ejecuta el servidor de salas, `zmp-server`. Es un único programa pequeño: no ejecuta el juego,
solo ordena y reenvía las entradas de todos y guarda las partidas de las salas, así que casi no usa CPU, memoria ni
ancho de banda. El anfitrión puede jugar en el mismo PC al mismo tiempo.

Si no puedes (o no quieres) cambiar los ajustes de tu router, usa en su lugar [Tailscale o ZeroTier](05-host-with-a-virtual-lan.md):
suele ser más fácil.

## 1. Ejecutar el servidor

1. Pon `zmp-server-<version>-windows-amd64.exe` en una carpeta propia, por ejemplo `C:\Games\ZMP-server`, y
   renómbralo `zmp-server.exe` si quieres.
2. Haz doble clic. Se abre una ventana de consola y muestra una línea como
   `zmp-server 0.6.0 protocol 4 listening on [::]:47100 (tcp and websocket)`. Mantén esa ventana abierta mientras
   juegas; si la cierras, el servidor se detiene.
3. La primera vez, el **Firewall de Windows** te pregunta si permitirlo: permítelo en **redes privadas** (y en redes
   públicas solo si tu red doméstica está marcada como pública).

El servidor escribe las partidas compartidas de las salas en una carpeta `data` junto a él. Para usar otro puerto u
otra carpeta, inícialo desde una terminal: `zmp-server.exe -listen :47100 -data C:\Games\ZMP-server\data`.
`zmp-server.exe -version` muestra su versión.

En Linux o en una Raspberry Pi: `./zmp-server-<version>-linux-amd64` (o `-linux-arm64`), con las mismas opciones. Para mantenerlo
en marcha en segundo plano, consulta [Hospedar en un VPS](06-host-on-a-vps.md).

## 2. Quién se conecta a qué

| Quién | Dirección del servidor que debe escribir |
|---|---|
| Tú, en el PC que ejecuta el servidor | `127.0.0.1` |
| Alguien en tu red doméstica (el mismo router) | la dirección local de tu PC, por ejemplo `192.168.0.20` (ver abajo) |
| Amigos por internet | tu dirección **pública**, después del paso 3 |

Puerto: `47100` en todos los casos. Tu dirección local: abre una terminal (Win+R, `cmd`) y ejecuta `ipconfig`; es la
"Dirección IPv4" de tu adaptador de red.

## 3. Abrir el puerto en tu router

Los amigos por internet solo pueden llegar a tu PC si tu router redirige el puerto hacia él:

1. Dale a tu PC una dirección local fija: en el router, busca la opción "reserva DHCP" / "asignación estática" y
   reserva para tu PC la dirección local actual (si no, puede cambiar y la redirección deja de funcionar).
2. Busca **Redirección de puertos** (también llamada "Servidor virtual", "NAT" o "Asignación de puertos") en los
   ajustes del router (normalmente en `http://192.168.0.1` o `http://192.168.1.1`; la dirección y la contraseña suelen
   estar en una pegatina del router).
3. Añade una regla: **TCP**, puerto externo **47100**, puerto interno **47100**, hacia la dirección local de tu PC.
4. Averigua tu dirección pública (busca "cuál es mi IP" en un navegador) y envíasela a tus amigos junto con el puerto
   y el nombre de la sala.
5. Pruébalo: pide a un amigo que se conecte. (Probar desde tu propio PC con tu dirección pública puede no funcionar en
   algunos routers; eso no significa que esté mal para los demás.)

Si no funciona:

- Si la dirección "WAN" o "Internet" del router es distinta de la dirección pública que muestra el navegador, o empieza
  por `100.64` hasta `100.127`, tu proveedor de internet usa **CGNAT** y la redirección de puertos no puede funcionar.
  Usa [Tailscale o ZeroTier](05-host-with-a-virtual-lan.md), o un [VPS](06-host-on-a-vps.md).
- Comprueba que la ventana del servidor sigue abierta y que el Firewall de Windows lo permitió.

## 4. Seguridad

- El servidor no tiene cuentas ni contraseñas: **cualquiera que conozca tu dirección, el puerto y el nombre de la sala
  puede entrar en esa sala**. Usa un nombre de sala que no sea fácil de adivinar y dáselo solo a las personas con las
  que juegas.
- Solo se abre el puerto `47100`, y solo `zmp-server` escucha en él. Nunca ejecuta el juego, nunca recibe de los
  jugadores otros archivos que no sean el estado del juego y nunca guarda nada fuera de su carpeta `data`.
- Cuando termines de jugar, cierra la ventana del servidor y quita la regla de redirección si no vas a hospedar de
  nuevo.

## 5. También jugadores de navegador

El mismo servidor y el mismo puerto atienden también a los jugadores de navegador (WebSocket). Para que tus amigos
jueguen en el navegador sin instalar nada, inicia el servidor con la versión para navegador: consulta [Modo
navegador](07-browser-mode.md).

## Cuántos jugadores y a qué distancia

Hasta **6 jugadores** por sala, y varias salas a la vez en el mismo servidor. Lockstep espera a la conexión más lenta,
así que se juega mejor cuando todos están en la misma región (el mismo país o países vecinos). El servidor adapta el
retraso de entrada a las conexiones cada pocos segundos.
