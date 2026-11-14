# 9. Plan B: open the port by hand on your router

**English** | [Español](../es/09-open-the-port-by-hand.md)

When you host, the game asks your router to open the port by itself. If the "From the internet" line says
**"Router without UPnP"** or **"The router answered but did not open the port"**, you can open it yourself in the
router's settings. It takes five minutes, once.

This does **not** help with **"Possible CGNAT"**: then the internet cannot reach your router at all. Use
[Tailscale or ZeroTier](05-host-with-a-virtual-lan.md), a [relay](11-relay.md) or a [VPS](06-host-on-a-vps.md).

## 1. Your PC's local address

Host once from the game: under the "From the internet" line, the Host tab shows "From a browser on your network:
http://**192.168.1.20**:47100/". That number is your PC's address on your home network. (Or open a terminal, run
`ipconfig` and look for the "IPv4 Address" of your network adapter.)

## 2. Make that address fixed

In the router, find the option called "DHCP reservation", "static lease" or "address reservation", and reserve that
address for your PC. Otherwise it may change one day and the rule below stops working.

## 3. Add the forwarding rule

1. Open the router's settings in a browser (usually `http://192.168.1.1` or similar; the address and
   password are often on a sticker on the router).
2. Find **Port forwarding** (also called "Virtual server", "NAT" or "Port mapping").
3. Add a rule: protocol **TCP**, external port **47100**, internal port **47100**, to your PC's local address.
4. Save.

If you host on another port in the Host tab, forward that one instead.

## 4. Tell your friends where to connect

The router does not tell the game about a rule you made by hand, so the invite code from the game's Host tab only
holds your home network addresses. Friends on the internet join **by address**:

- **Server**: your **public address** (search "what is my IP" in a browser, or look at the router's status page);
- **Room** and **Password**: shown in your Host tab;
- they press **Connect** in **Join > By address**.

With the [double-click server](04-host-on-your-pc.md) you can put your public address (or a domain name) in
`public_address` in `zmp-server.toml`, and set `port_mapping = false`: then the invite code holds it, and friends
just paste the code.

When a friend joins from the internet, the "From the internet" line says "Reachable from the internet".

## If it still does not work

- If the router's "WAN" or "Internet" address is different from the public address the browser shows, or starts with
  `100.64` to `100.127`, your provider uses **CGNAT** and port forwarding cannot work: use a plan B from the list
  above.
- Check that Windows Firewall allowed `zmp-server` (on public networks too, if your network is set as public).
- Test from outside: open `http://<your public address>:47100/healthz` from a phone on mobile data (not your Wi-Fi).
  It should answer `zmp-server ... protocol 5`. Testing from your own PC with your public address may not work on
  some routers; that does not mean it is broken for others.

## Safety

- Only port 47100 is opened, and only `zmp-server` listens on it. It never runs the game, never receives files from
  players other than the game's state, and never stores anything outside its data folder.
- The room has a password, and after a few wrong tries an address has to wait.
- When you will not host any more, remove the forwarding rule.
