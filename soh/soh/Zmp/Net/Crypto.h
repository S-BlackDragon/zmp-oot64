#pragma once

#include <array>
#include <cstdint>
#include <string>
#include <vector>

// SHA-256, HMAC-SHA256 and PBKDF2-HMAC-SHA256 (FIPS 180-4, RFC 2104, RFC 8018) in portable C++, the same code in the
// native and the web build, for the room password of protocol 5 and the invite code's checksum (docs/PROTOCOLO_5.md
// sections 3 and 7). The harness checks them against the server's vectors.
namespace Zmp::Crypto {

using Bytes = std::vector<uint8_t>;
using Digest = std::array<uint8_t, 32>;

Digest Sha256(const uint8_t* data, size_t size);
Digest Sha256(const Bytes& data);
Digest HmacSha256(const Bytes& key, const Bytes& msg);
Digest Pbkdf2Sha256(const Bytes& password, const Bytes& salt, uint32_t iterations);

Bytes ToBytes(const std::string& s);
std::string Hex(const uint8_t* data, size_t size);
template <size_t N> std::string Hex(const std::array<uint8_t, N>& d) {
    return Hex(d.data(), N);
}
bool FromHex(const std::string& hex, Bytes& out);
// Random bytes from the system (BCryptGenRandom / getrandom / the browser's crypto.getRandomValues).
Bytes Random(size_t n);

// docs/PROTOCOLO_5.md 3.
std::string NormalizePassword(const std::string& p);
Digest RoomKey(const std::string& room, const std::string& password);
Digest ServerKey(const std::string& password);
Digest Proof(const Digest& key, const std::string& kind, const Bytes& serverNonce, const Bytes& clientNonce,
             const std::string& room, const std::string& name);
Digest Confirm(const Digest& key, const std::string& kind, const Bytes& serverNonce, const Bytes& clientNonce);

} // namespace Zmp::Crypto
