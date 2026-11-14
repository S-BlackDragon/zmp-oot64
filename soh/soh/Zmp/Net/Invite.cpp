#include "Invite.h"

#include <algorithm>
#include <cctype>
#include <cstdio>
#include <cstdlib>
#include <cstring>

#include "Crypto.h"

namespace Zmp::Invite {

namespace {

const char* kAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
constexpr uint8_t kVersion = 1;
constexpr uint8_t kFlagPort = 0x01, kFlagPassword = 0x02, kFlagHint = 0x04;
constexpr uint8_t kAddrHost = 1, kAddrUrl = 2, kAddrV4 = 4, kAddrV6 = 6;

std::vector<uint8_t> Checksum(const std::vector<uint8_t>& body) {
    std::vector<uint8_t> msg = Crypto::ToBytes(std::string("zmp-invite") + '\0');
    msg.insert(msg.end(), body.begin(), body.end());
    Crypto::Digest d = Crypto::Sha256(msg);
    return std::vector<uint8_t>(d.begin(), d.begin() + 4);
}

std::string Lower(std::string s) {
    std::transform(s.begin(), s.end(), s.begin(), [](unsigned char c) { return (char)std::tolower(c); });
    return s;
}

bool Printable(const std::string& s) {
    return std::all_of(s.begin(), s.end(), [](unsigned char c) { return c > ' ' && c < 0x7f; });
}

bool ValidHost(const std::string& h) {
    if (h.empty() || h.size() > 253) {
        return false;
    }
    size_t i = 0;
    while (i <= h.size()) {
        size_t j = h.find('.', i);
        if (j == std::string::npos) {
            j = h.size();
        }
        std::string label = h.substr(i, j - i);
        if (label.empty() || label.size() > 63 || label.front() == '-' || label.back() == '-') {
            return false;
        }
        for (char c : label) {
            if (!(std::isalnum((unsigned char)c) || c == '-')) {
                return false;
            }
        }
        i = j + 1;
    }
    return true;
}

bool ValidUtf8(const std::string& s) {
    size_t i = 0, n = s.size();
    while (i < n) {
        unsigned char c = (unsigned char)s[i];
        int extra;
        if (c < 0x80) {
            extra = 0;
        } else if ((c >> 5) == 6) {
            extra = 1;
        } else if ((c >> 4) == 14) {
            extra = 2;
        } else if ((c >> 3) == 30) {
            extra = 3;
        } else {
            return false;
        }
        if (i + extra >= n && extra > 0) {
            return false;
        }
        for (int k = 1; k <= extra; k++) {
            if (((unsigned char)s[i + k] >> 6) != 2) {
                return false;
            }
        }
        i += (size_t)extra + 1;
    }
    return true;
}

bool ParseV4(const std::string& s, uint8_t out[4]) {
    int a, b, c, d;
    char extra;
    if (sscanf(s.c_str(), "%d.%d.%d.%d%c", &a, &b, &c, &d, &extra) != 4) {
        return false;
    }
    for (int v : { a, b, c, d }) {
        if (v < 0 || v > 255) {
            return false;
        }
    }
    out[0] = (uint8_t)a;
    out[1] = (uint8_t)b;
    out[2] = (uint8_t)c;
    out[3] = (uint8_t)d;
    return std::count(s.begin(), s.end(), '.') == 3 &&
           std::all_of(s.begin(), s.end(), [](char ch) { return ch == '.' || (ch >= '0' && ch <= '9'); });
}

// RFC 4291 text form (with "::"); an embedded IPv4 tail is accepted.
bool ParseV6(const std::string& s, uint8_t out[16]) {
    if (s.find(':') == std::string::npos || s.find('%') != std::string::npos) {
        return false;
    }
    std::vector<uint16_t> head, tail;
    bool gap = false;
    size_t dbl = s.find("::");
    auto parseGroups = [](const std::string& part, std::vector<uint16_t>& groups) -> bool {
        if (part.empty()) {
            return true;
        }
        size_t i = 0;
        while (i <= part.size()) {
            size_t j = part.find(':', i);
            if (j == std::string::npos) {
                j = part.size();
            }
            std::string g = part.substr(i, j - i);
            if (g.find('.') != std::string::npos && j == part.size()) {
                uint8_t v4[4];
                if (!ParseV4(g, v4)) {
                    return false;
                }
                groups.push_back((uint16_t)(v4[0] << 8 | v4[1]));
                groups.push_back((uint16_t)(v4[2] << 8 | v4[3]));
                return true;
            }
            if (g.empty() || g.size() > 4 || !std::all_of(g.begin(), g.end(), ::isxdigit)) {
                return false;
            }
            groups.push_back((uint16_t)strtoul(g.c_str(), nullptr, 16));
            i = j + 1;
        }
        return true;
    };
    if (dbl != std::string::npos) {
        gap = true;
        if (s.find("::", dbl + 1) != std::string::npos) {
            return false;
        }
        if (!parseGroups(s.substr(0, dbl), head) || !parseGroups(s.substr(dbl + 2), tail)) {
            return false;
        }
    } else if (!parseGroups(s, head)) {
        return false;
    }
    size_t total = head.size() + tail.size();
    if ((gap && total > 7) || (!gap && total != 8)) {
        return false;
    }
    std::vector<uint16_t> all = head;
    all.insert(all.end(), 8 - total, 0);
    all.insert(all.end(), tail.begin(), tail.end());
    for (int k = 0; k < 8; k++) {
        out[k * 2] = (uint8_t)(all[k] >> 8);
        out[k * 2 + 1] = (uint8_t)all[k];
    }
    return true;
}

// Go's netip.Addr.String() for IPv6: lowercase, no leading zeros, the longest run (2+) of zero groups as "::";
// an IPv4-mapped address as ::ffff:a.b.c.d.
std::string FormatV6(const uint8_t b[16]) {
    uint16_t g[8];
    for (int k = 0; k < 8; k++) {
        g[k] = (uint16_t)(b[k * 2] << 8 | b[k * 2 + 1]);
    }
    bool mapped = g[0] == 0 && g[1] == 0 && g[2] == 0 && g[3] == 0 && g[4] == 0 && g[5] == 0xffff;
    if (mapped) {
        char t[40];
        snprintf(t, sizeof(t), "::ffff:%d.%d.%d.%d", b[12], b[13], b[14], b[15]);
        return t;
    }
    int bestStart = -1, bestLen = 0;
    for (int k = 0; k < 8;) {
        if (g[k] != 0) {
            k++;
            continue;
        }
        int j = k;
        while (j < 8 && g[j] == 0) {
            j++;
        }
        if (j - k > bestLen && j - k >= 2) {
            bestStart = k;
            bestLen = j - k;
        }
        k = j;
    }
    std::string out;
    for (int k = 0; k < 8; k++) {
        if (k == bestStart) {
            out += "::";
            k += bestLen - 1;
            continue;
        }
        if (!out.empty() && out.back() != ':') {
            out += ':';
        }
        char t[8];
        snprintf(t, sizeof(t), "%x", g[k]);
        out += t;
    }
    return out;
}

std::string Base32(const std::vector<uint8_t>& b) {
    std::string s;
    uint32_t acc = 0;
    int bits = 0;
    for (uint8_t x : b) {
        acc = acc << 8 | x;
        bits += 8;
        while (bits >= 5) {
            bits -= 5;
            s.push_back(kAlphabet[(acc >> bits) & 31]);
        }
    }
    if (bits > 0) {
        s.push_back(kAlphabet[(acc << (5 - bits)) & 31]);
    }
    return s;
}

bool UnBase32(const std::string& s, std::vector<uint8_t>& out, std::string& error) {
    if ((s.size() * 5) % 8 >= 5) {
        error = "length";
        return false;
    }
    out.clear();
    uint32_t acc = 0;
    int bits = 0;
    for (char c : s) {
        const char* p = strchr(kAlphabet, c);
        if (p == nullptr || c == '\0') {
            error = "character";
            return false;
        }
        acc = acc << 5 | (uint32_t)(p - kAlphabet);
        bits += 5;
        if (bits >= 8) {
            bits -= 8;
            out.push_back((uint8_t)(acc >> bits));
        }
    }
    if (bits > 0 && (acc & ((1u << bits) - 1)) != 0) {
        error = "length";
        return false;
    }
    return true;
}

bool Decode(const std::vector<uint8_t>& b, Code& inv, std::string& error) {
    inv = Code();
    if (b.empty()) {
        error = "empty";
        return false;
    }
    if (b[0] != kVersion) {
        error = b[0] > kVersion ? "version" : "malformed";
        return false;
    }
    if (b.size() < 2 + 1 + 3 + 2 + 4) {
        error = "length";
        return false;
    }
    std::vector<uint8_t> body(b.begin(), b.end() - 4);
    if (Checksum(body) != std::vector<uint8_t>(b.end() - 4, b.end())) {
        error = "checksum";
        return false;
    }
    uint8_t flags = body[1];
    if ((flags & ~(kFlagPort | kFlagPassword | kFlagHint)) != 0 || ((flags & kFlagPassword) && (flags & kFlagHint))) {
        error = "version";
        return false;
    }
    size_t p = 2;
    auto need = [&](size_t n) { return body.size() - p >= n; };
    if (flags & kFlagPort) {
        if (!need(2)) {
            error = "malformed";
            return false;
        }
        inv.port = body[p] << 8 | body[p + 1];
        p += 2;
        if (inv.port == 0) {
            error = "invalid";
            return false;
        }
    }
    if (!need(1)) {
        error = "malformed";
        return false;
    }
    int n = body[p++];
    if (n < 1 || n > 4) {
        error = "invalid";
        return false;
    }
    for (int i = 0; i < n; i++) {
        if (!need(1)) {
            error = "malformed";
            return false;
        }
        uint8_t typ = body[p++];
        if (typ == kAddrV4) {
            if (!need(4)) {
                error = "malformed";
                return false;
            }
            char t[20];
            snprintf(t, sizeof(t), "%d.%d.%d.%d", body[p], body[p + 1], body[p + 2], body[p + 3]);
            inv.addresses.push_back(t);
            p += 4;
        } else if (typ == kAddrV6) {
            if (!need(16)) {
                error = "malformed";
                return false;
            }
            inv.addresses.push_back(FormatV6(&body[p]));
            p += 16;
        } else if (typ == kAddrHost || typ == kAddrUrl) {
            if (!need(1) || !need(1 + (size_t)body[p])) {
                error = "malformed";
                return false;
            }
            std::string s((const char*)&body[p + 1], body[p]);
            p += 1 + (size_t)body[p];
            std::string l = Lower(s);
            if ((typ == kAddrHost && !ValidHost(s)) ||
                (typ == kAddrUrl && (!Printable(s) || !(l.rfind("ws://", 0) == 0 || l.rfind("wss://", 0) == 0)))) {
                error = "invalid";
                return false;
            }
            inv.addresses.push_back(s);
        } else {
            error = "version";
            return false;
        }
    }
    auto str = [&](size_t max, std::string& out) -> bool {
        if (!need(1) || !need(1 + (size_t)body[p])) {
            error = "malformed";
            return false;
        }
        out.assign((const char*)&body[p + 1], body[p]);
        p += 1 + (size_t)body[p];
        if (out.empty() || out.size() > max || !ValidUtf8(out)) {
            error = "invalid";
            return false;
        }
        return true;
    };
    if (!str(64, inv.room)) {
        return false;
    }
    if (flags & (kFlagPassword | kFlagHint)) {
        std::string s;
        if (!str(64, s)) {
            return false;
        }
        (flags & kFlagPassword ? inv.password : inv.hint) = s;
    }
    if (p != body.size()) {
        error = "malformed";
        return false;
    }
    return true;
}

bool EncodeAddress(std::string a, std::vector<uint8_t>& b, std::string& error) {
    while (!a.empty() && std::isspace((unsigned char)a.back())) {
        a.pop_back();
    }
    while (!a.empty() && std::isspace((unsigned char)a.front())) {
        a.erase(a.begin());
    }
    if (a.size() >= 2 && a.front() == '[' && a.back() == ']') {
        a = a.substr(1, a.size() - 2);
    }
    uint8_t v4[4], v6[16];
    if (ParseV4(a, v4)) {
        b.push_back(kAddrV4);
        b.insert(b.end(), v4, v4 + 4);
        return true;
    }
    if (ParseV6(a, v6)) {
        bool mapped = std::all_of(v6, v6 + 10, [](uint8_t x) { return x == 0; }) && v6[10] == 0xff && v6[11] == 0xff;
        if (mapped) {
            b.push_back(kAddrV4);
            b.insert(b.end(), v6 + 12, v6 + 16);
        } else {
            b.push_back(kAddrV6);
            b.insert(b.end(), v6, v6 + 16);
        }
        return true;
    }
    std::string l = Lower(a);
    if (l.rfind("ws://", 0) == 0 || l.rfind("wss://", 0) == 0) {
        if (a.size() > 255 || !Printable(a)) {
            error = "invalid";
            return false;
        }
        b.push_back(kAddrUrl);
        b.push_back((uint8_t)a.size());
        b.insert(b.end(), a.begin(), a.end());
        return true;
    }
    if (!ValidHost(a)) {
        error = "invalid";
        return false;
    }
    b.push_back(kAddrHost);
    b.push_back((uint8_t)a.size());
    b.insert(b.end(), a.begin(), a.end());
    return true;
}

} // namespace

std::string Normalize(const std::string& textIn) {
    std::string t = textIn;
    while (!t.empty() && std::isspace((unsigned char)t.back())) {
        t.pop_back();
    }
    size_t a = 0;
    while (a < t.size() && std::isspace((unsigned char)t[a])) {
        a++;
    }
    t = t.substr(a);
    std::string low = Lower(t);
    for (const char* p : { "zmp://join/", "zmp://", "zmp:" }) {
        if (low.rfind(p, 0) == 0) {
            t = t.substr(strlen(p));
            break;
        }
    }
    std::string s;
    for (char c : t) {
        if (c == '-' || c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '/' || c == '.') {
            continue;
        }
        if (c >= 'a' && c <= 'z') {
            c = (char)(c - 'a' + 'A');
        }
        if (c == 'O') {
            c = '0';
        } else if (c == 'I' || c == 'L') {
            c = '1';
        }
        s.push_back(c);
    }
    if (s.rfind("ZMP", 0) == 0) {
        s = s.substr(3);
    }
    return s;
}

bool Parse(const std::string& text, Code& out, std::string& error) {
    std::string s = Normalize(text);
    if (s.empty()) {
        error = "empty";
        return false;
    }
    std::vector<uint8_t> b;
    if (!UnBase32(s, b, error)) {
        return false;
    }
    return Decode(b, out, error);
}

bool Format(const Code& inv, std::string& text, std::string& link, std::string& error) {
    text.clear();
    link.clear();
    if (inv.addresses.empty() || inv.addresses.size() > 4 || inv.room.empty() || inv.room.size() > 64 ||
        !ValidUtf8(inv.room) || (!inv.password.empty() && !inv.hint.empty()) || (inv.password + inv.hint).size() > 64 ||
        inv.port < 0 || inv.port > 65535) {
        error = "invalid";
        return false;
    }
    std::vector<uint8_t> b = { kVersion, 0 };
    if (inv.port != 0 && inv.port != 47100) {
        b[1] |= kFlagPort;
        b.push_back((uint8_t)(inv.port >> 8));
        b.push_back((uint8_t)inv.port);
    }
    std::string secret = inv.password + inv.hint;
    if (!inv.password.empty()) {
        b[1] |= kFlagPassword;
    } else if (!inv.hint.empty()) {
        b[1] |= kFlagHint;
    }
    b.push_back((uint8_t)inv.addresses.size());
    for (auto& a : inv.addresses) {
        if (!EncodeAddress(a, b, error)) {
            return false;
        }
    }
    b.push_back((uint8_t)inv.room.size());
    b.insert(b.end(), inv.room.begin(), inv.room.end());
    if (!secret.empty()) {
        b.push_back((uint8_t)secret.size());
        b.insert(b.end(), secret.begin(), secret.end());
    }
    std::vector<uint8_t> sum = Checksum(b);
    b.insert(b.end(), sum.begin(), sum.end());
    std::string s = Base32(b);
    text = "ZMP";
    for (size_t i = 0; i < s.size(); i += 4) {
        text += "-" + s.substr(i, 4);
    }
    link = "zmp://join/" + s;
    return true;
}

bool LooksLikeCode(const std::string& text) {
    std::string low = Lower(text);
    size_t a = low.find_first_not_of(" \t\r\n");
    if (a == std::string::npos) {
        return false;
    }
    low = low.substr(a);
    return low.rfind("zmp:", 0) == 0 || low.rfind("zmp-", 0) == 0 || low.rfind("zmp ", 0) == 0;
}

std::string AddressFor(const Code& code, size_t index) {
    if (index >= code.addresses.size()) {
        return "";
    }
    const std::string& a = code.addresses[index];
    std::string l = Lower(a);
    if (l.rfind("ws://", 0) == 0 || l.rfind("wss://", 0) == 0) {
        return a;
    }
    std::string host = a.find(':') != std::string::npos ? "[" + a + "]" : a;
    return host + ":" + std::to_string(code.port);
}

} // namespace Zmp::Invite
