#pragma once

// Native transports of the ZMP client (docs/PRODUCTO_PUBLICO.md 5.1): plain TCP (uint32 LE length + MessagePack) and
// WebSocket ws:// / wss:// (one binary message per MessagePack body, D-239) through the system's WinHTTP, so TLS is
// Windows' own (Schannel, the system's certificate store, certificate checks on). The web build has its own
// transport in ZmpClient.cpp (the browser's WebSocket).
#ifndef __EMSCRIPTEN__

#include <cstdint>
#include <memory>
#include <string>
#include <vector>

#include <nlohmann/json.hpp>

#include "Address.h"

namespace Zmp {

class Conn {
  public:
    virtual ~Conn() = default;
    // One frame as EncodeFrame makes it (length prefix + body). Any thread; one sender at a time.
    virtual bool SendFrame(const std::vector<uint8_t>& frame) = 0;
    // Waits up to `ms` for data and appends the complete messages to `out`. False once the connection is over (the
    // messages that arrived before are still appended). Throws on a malformed message.
    virtual bool Poll(int ms, std::vector<nlohmann::json>& out) = 0;
    virtual void Close() = 0;
};

struct DialResult {
    std::unique_ptr<Conn> conn;
    std::string reason; // "unreachable", "tls_error", "not_zmp_server", "unsupported" (when conn is null)
    std::string detail; // a short English description of the system error, for the log
};

// Opens a connection to `a`. timeoutMs bounds the TCP connect (the WebSocket handshake has its own bound).
DialResult Dial(const Address& a, int timeoutMs);

} // namespace Zmp

#endif
