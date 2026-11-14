# 5. Plan B: Tailscale or ZeroTier (no router changes)

**English** | [Español](../es/05-host-with-a-virtual-lan.md)

A virtual LAN puts your PCs on a private network over the internet, as if you were in the same house. Nothing is
opened on your router, it works behind CGNAT, and only the people you invite can reach you. Both services below have
free plans for small groups. ZMP does not depend on either: any VPN or virtual LAN that lets your friends reach your
PC on TCP port 47100 works the same way (WireGuard, Radmin VPN, Hamachi...).

## Host

Host as usual: **Host** tab, "My local network and whoever has the code", **Host and play**
([main guide](00-host-and-send-the-code.md)). The "From the internet" line is about your real router, so here it may
say anything: ignore it. (A PC that only hosts can run the [double-click server](04-host-on-your-pc.md) instead.)

Then give your friends three things: your **virtual LAN address** (below), the **room** and the **room password**
(both shown in the Host tab).

The invite code holds your home network addresses, not always the virtual one, so friends join **by address**.

## Friends

**Join** tab, section **By address**:

- **Server**: the host's virtual LAN address (port 47100 is the default, no need to add it);
- **Room** and **Password**: what the host gave you;
- press **Connect**.

## Tailscale

1. Everyone installs Tailscale from <https://tailscale.com/download> and signs in.
2. The host invites the friends. Two ways:
   - **Share only the host PC** (recommended): in the Tailscale admin console, open the host machine's menu, choose
     **Share...** and send the invite link to each friend. They see that PC and nothing else of your network.
   - Or invite them to your tailnet as users (they would see every device you have in it).
3. The host's address: the Tailscale icon or `tailscale ip -4` shows an address like `100.101.102.103`; with
   MagicDNS on, a name like `my-pc.tailnet-name.ts.net` also works.

## ZeroTier

1. The host creates a network at <https://my.zerotier.com> (free account) and copies its **Network ID** (16
   characters).
2. Everyone installs ZeroTier from <https://www.zerotier.com/download/>, then **Join New Network** with that ID.
3. The host, in the network's page, ticks **Auth** for each member that joined (on a private network, which is the
   default).
4. The host's address is the **Managed IP** shown next to the host PC in that page (for example `10.147.17.5`).

## If a friend cannot connect

- Check that the host's Windows Firewall allowed `zmp-server`. Virtual LAN traffic counts as a private network on
  most PCs; if Windows shows that network as public, allow the server on public networks too.
- Both PCs must show as connected in Tailscale or ZeroTier.

## Browser players

Browser players can use the same virtual LAN with a server that serves the page itself
(`zmp-server -web <folder>`): they open `http://<host address>:47100/` (plain `http`, which is fine inside the
private network). See [Browser mode](07-browser-mode.md). They still need Tailscale or ZeroTier running.
