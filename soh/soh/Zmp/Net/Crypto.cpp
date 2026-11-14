#include "Crypto.h"

#include <algorithm>
#include <cstring>

#if defined(__EMSCRIPTEN__)
#include <emscripten.h>
#elif defined(_WIN32)
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#include <bcrypt.h>
#pragma comment(lib, "bcrypt.lib")
#else
#include <fstream>
#endif

namespace Zmp::Crypto {

namespace {

constexpr uint32_t kK[64] = {
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
};

inline uint32_t Rotr(uint32_t x, int n) {
    return (x >> n) | (x << (32 - n));
}

struct Sha {
    uint32_t h[8] = { 0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19 };
    uint8_t block[64];
    size_t used = 0;
    uint64_t total = 0;

    void Compress(const uint8_t* p) {
        uint32_t w[64];
        for (int i = 0; i < 16; i++) {
            w[i] = (uint32_t)p[i * 4] << 24 | (uint32_t)p[i * 4 + 1] << 16 | (uint32_t)p[i * 4 + 2] << 8 | p[i * 4 + 3];
        }
        for (int i = 16; i < 64; i++) {
            uint32_t s0 = Rotr(w[i - 15], 7) ^ Rotr(w[i - 15], 18) ^ (w[i - 15] >> 3);
            uint32_t s1 = Rotr(w[i - 2], 17) ^ Rotr(w[i - 2], 19) ^ (w[i - 2] >> 10);
            w[i] = w[i - 16] + s0 + w[i - 7] + s1;
        }
        uint32_t a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7];
        for (int i = 0; i < 64; i++) {
            uint32_t s1 = Rotr(e, 6) ^ Rotr(e, 11) ^ Rotr(e, 25);
            uint32_t ch = (e & f) ^ (~e & g);
            uint32_t t1 = hh + s1 + ch + kK[i] + w[i];
            uint32_t s0 = Rotr(a, 2) ^ Rotr(a, 13) ^ Rotr(a, 22);
            uint32_t mj = (a & b) ^ (a & c) ^ (b & c);
            uint32_t t2 = s0 + mj;
            hh = g;
            g = f;
            f = e;
            e = d + t1;
            d = c;
            c = b;
            b = a;
            a = t1 + t2;
        }
        h[0] += a;
        h[1] += b;
        h[2] += c;
        h[3] += d;
        h[4] += e;
        h[5] += f;
        h[6] += g;
        h[7] += hh;
    }

    void Update(const uint8_t* p, size_t n) {
        total += n;
        while (n > 0) {
            size_t take = std::min(n, (size_t)64 - used);
            memcpy(block + used, p, take);
            used += take;
            p += take;
            n -= take;
            if (used == 64) {
                Compress(block);
                used = 0;
            }
        }
    }

    Digest Final() {
        uint64_t bits = total * 8;
        uint8_t pad = 0x80;
        Update(&pad, 1);
        uint8_t zero = 0;
        while (used != 56) {
            Update(&zero, 1);
        }
        uint8_t len[8];
        for (int i = 0; i < 8; i++) {
            len[i] = (uint8_t)(bits >> (56 - 8 * i));
        }
        Update(len, 8);
        Digest d;
        for (int i = 0; i < 8; i++) {
            d[i * 4] = (uint8_t)(h[i] >> 24);
            d[i * 4 + 1] = (uint8_t)(h[i] >> 16);
            d[i * 4 + 2] = (uint8_t)(h[i] >> 8);
            d[i * 4 + 3] = (uint8_t)h[i];
        }
        return d;
    }
};

// HMAC with the key's inner and outer pads computed once (PBKDF2 runs 10 000 of them).
struct Hmac {
    Sha inner, outer;
    explicit Hmac(const Bytes& keyIn) {
        uint8_t key[64] = {};
        if (keyIn.size() > 64) {
            Digest k = Sha256(keyIn);
            memcpy(key, k.data(), 32);
        } else if (!keyIn.empty()) {
            memcpy(key, keyIn.data(), keyIn.size());
        }
        uint8_t ip[64], op[64];
        for (int i = 0; i < 64; i++) {
            ip[i] = key[i] ^ 0x36;
            op[i] = key[i] ^ 0x5c;
        }
        inner.Update(ip, 64);
        outer.Update(op, 64);
    }
    Digest Mac(const uint8_t* msg, size_t n) const {
        Sha in = inner;
        in.Update(msg, n);
        Digest d = in.Final();
        Sha out = outer;
        out.Update(d.data(), d.size());
        return out.Final();
    }
};

void Append(Bytes& b, const std::string& s) {
    b.insert(b.end(), s.begin(), s.end());
}

} // namespace

Digest Sha256(const uint8_t* data, size_t size) {
    Sha s;
    s.Update(data, size);
    return s.Final();
}

Digest Sha256(const Bytes& data) {
    return Sha256(data.data(), data.size());
}

Digest HmacSha256(const Bytes& key, const Bytes& msg) {
    return Hmac(key).Mac(msg.data(), msg.size());
}

Digest Pbkdf2Sha256(const Bytes& password, const Bytes& salt, uint32_t iterations) {
    // One 32-byte block (dkLen = hLen): T1 = U1 ^ ... ^ Uc, U1 = PRF(P, S || INT(1)).
    Hmac prf(password);
    Bytes first = salt;
    first.push_back(0);
    first.push_back(0);
    first.push_back(0);
    first.push_back(1);
    Digest u = prf.Mac(first.data(), first.size());
    Digest t = u;
    for (uint32_t i = 1; i < iterations; i++) {
        u = prf.Mac(u.data(), u.size());
        for (int k = 0; k < 32; k++) {
            t[k] ^= u[k];
        }
    }
    return t;
}

Bytes ToBytes(const std::string& s) {
    return Bytes(s.begin(), s.end());
}

std::string Hex(const uint8_t* data, size_t size) {
    static const char* digits = "0123456789abcdef";
    std::string out;
    out.reserve(size * 2);
    for (size_t i = 0; i < size; i++) {
        out.push_back(digits[data[i] >> 4]);
        out.push_back(digits[data[i] & 15]);
    }
    return out;
}

bool FromHex(const std::string& hex, Bytes& out) {
    out.clear();
    if (hex.size() % 2 != 0) {
        return false;
    }
    auto val = [](char c) -> int {
        if (c >= '0' && c <= '9') {
            return c - '0';
        }
        if (c >= 'a' && c <= 'f') {
            return c - 'a' + 10;
        }
        if (c >= 'A' && c <= 'F') {
            return c - 'A' + 10;
        }
        return -1;
    };
    for (size_t i = 0; i < hex.size(); i += 2) {
        int a = val(hex[i]), b = val(hex[i + 1]);
        if (a < 0 || b < 0) {
            return false;
        }
        out.push_back((uint8_t)(a * 16 + b));
    }
    return true;
}

Bytes Random(size_t n) {
    Bytes b(n);
#if defined(__EMSCRIPTEN__)
    for (size_t i = 0; i < n; i++) {
        b[i] = (uint8_t)EM_ASM_INT({ return crypto.getRandomValues(new Uint8Array(1))[0]; });
    }
#elif defined(_WIN32)
    BCryptGenRandom(nullptr, b.data(), (ULONG)n, BCRYPT_USE_SYSTEM_PREFERRED_RNG);
#else
    std::ifstream f("/dev/urandom", std::ios::binary);
    f.read((char*)b.data(), (std::streamsize)n);
#endif
    return b;
}

std::string NormalizePassword(const std::string& p) {
    size_t a = 0, e = p.size();
    auto space = [](char c) { return c == ' ' || c == '\t' || c == '\r' || c == '\n'; };
    while (a < e && space(p[a])) {
        a++;
    }
    while (e > a && space(p[e - 1])) {
        e--;
    }
    return p.substr(a, e - a);
}

Digest RoomKey(const std::string& room, const std::string& password) {
    Bytes salt = ToBytes(std::string("zmp-room-v1") + '\0' + room);
    return Pbkdf2Sha256(ToBytes(NormalizePassword(password)), salt, 10000);
}

Digest ServerKey(const std::string& password) {
    return Pbkdf2Sha256(ToBytes(NormalizePassword(password)), ToBytes("zmp-server-v1"), 10000);
}

Digest Proof(const Digest& key, const std::string& kind, const Bytes& serverNonce, const Bytes& clientNonce,
             const std::string& room, const std::string& name) {
    Bytes msg;
    Append(msg, std::string("zmp-auth-v1") + '\0' + kind + '\0');
    msg.insert(msg.end(), serverNonce.begin(), serverNonce.end());
    msg.insert(msg.end(), clientNonce.begin(), clientNonce.end());
    Append(msg, room + '\0' + name);
    return HmacSha256(Bytes(key.begin(), key.end()), msg);
}

Digest Confirm(const Digest& key, const std::string& kind, const Bytes& serverNonce, const Bytes& clientNonce) {
    Bytes msg;
    Append(msg, std::string("zmp-auth-v1") + '\0' + kind + "-ok" + '\0');
    msg.insert(msg.end(), serverNonce.begin(), serverNonce.end());
    msg.insert(msg.end(), clientNonce.begin(), clientNonce.end());
    return HmacSha256(Bytes(key.begin(), key.end()), msg);
}

} // namespace Zmp::Crypto
