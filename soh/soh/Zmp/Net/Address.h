#pragma once

#include <cstdint>
#include <string>

namespace Zmp {

// One address grammar for the desktop and the web client (docs/PRODUCTO_PUBLICO.md 5.1, interface I2; the cases are
// in tools/harness/data/address_cases.json and the harness checks this parser against them):
//   host, host:port, IPv4, [IPv6], [IPv6]:port, bare IPv6  -> TCP (the default port when there is none)
//   host:443                                                -> wss://host/
//   ws://host[:port][/path], wss://host[:port][/path]       -> as written (ports 80 / 443 by default)
//   http://... / https://...                                -> the same as ws:// / wss://
// A bare domain name (no scheme, no port, not an IP literal) may fall back to wss://name/ when TCP does not connect.
struct Address {
    enum class Kind { Tcp, Ws, Wss };
    bool ok = false;
    std::string error; // why it is not an address (code: "empty", "bad_port", "bad_host", "bad_scheme")
    Kind kind = Kind::Tcp;
    std::string host; // without brackets for IPv6
    uint16_t port = 0;
    std::string path = "/"; // WebSocket only
    bool bareDomain = false;

    // The WebSocket URL of this address (a TCP address as ws://host:port/, what the browser connects to).
    std::string Url() const;
    // How the address is shown and stored: "host:port" for TCP, the URL for WebSocket.
    std::string Text() const;
    static const char* KindName(Kind k);
};

Address ParseAddress(const std::string& text, uint16_t defaultPort);

// The wss:// address a bare domain falls back to.
Address WssFallback(const Address& a);

} // namespace Zmp
