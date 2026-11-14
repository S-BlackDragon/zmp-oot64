#include "Address.h"

#include <algorithm>
#include <cctype>

namespace Zmp {

static std::string Trim(const std::string& s) {
    size_t a = 0, b = s.size();
    while (a < b && std::isspace((unsigned char)s[a])) {
        a++;
    }
    while (b > a && std::isspace((unsigned char)s[b - 1])) {
        b--;
    }
    return s.substr(a, b - a);
}

static std::string Lower(std::string s) {
    std::transform(s.begin(), s.end(), s.begin(), [](unsigned char c) { return (char)std::tolower(c); });
    return s;
}

static bool ParsePort(const std::string& s, uint16_t* out) {
    if (s.empty() || s.size() > 5) {
        return false;
    }
    unsigned v = 0;
    for (char c : s) {
        if (c < '0' || c > '9') {
            return false;
        }
        v = v * 10 + (unsigned)(c - '0');
    }
    if (v == 0 || v > 65535) {
        return false;
    }
    *out = (uint16_t)v;
    return true;
}

static bool IsIPv4(const std::string& h) {
    int parts = 0;
    size_t i = 0;
    while (i <= h.size()) {
        size_t j = h.find('.', i);
        if (j == std::string::npos) {
            j = h.size();
        }
        std::string p = h.substr(i, j - i);
        if (p.empty() || p.size() > 3 || !std::all_of(p.begin(), p.end(), ::isdigit) || std::stoi(p) > 255) {
            return false;
        }
        parts++;
        i = j + 1;
    }
    return parts == 4;
}

static bool GoodHostChars(const std::string& h) {
    if (h.empty()) {
        return false;
    }
    for (char c : h) {
        if (!(std::isalnum((unsigned char)c) || c == '-' || c == '.' || c == '_')) {
            return false;
        }
    }
    return true;
}

static bool GoodV6(const std::string& h) {
    if (h.size() < 2) {
        return false;
    }
    for (char c : h) {
        if (!(std::isxdigit((unsigned char)c) || c == ':' || c == '.' || c == '%')) {
            return false;
        }
    }
    return h.find(':') != std::string::npos;
}

// "host", "host:port", "[v6]", "[v6]:port", "v6" -> host and port (0 when absent). False on a malformed one.
static bool SplitHostPort(const std::string& s, std::string* host, uint16_t* port, bool* v6, std::string* err) {
    *port = 0;
    *v6 = false;
    if (!s.empty() && s[0] == '[') {
        size_t close = s.find(']');
        if (close == std::string::npos) {
            *err = "bad_host";
            return false;
        }
        *host = s.substr(1, close - 1);
        *v6 = true;
        std::string rest = s.substr(close + 1);
        if (!GoodV6(*host)) {
            *err = "bad_host";
            return false;
        }
        if (rest.empty()) {
            return true;
        }
        if (rest[0] != ':' || !ParsePort(rest.substr(1), port)) {
            *err = "bad_port";
            return false;
        }
        return true;
    }
    size_t colons = std::count(s.begin(), s.end(), ':');
    if (colons > 1) {
        // a bare IPv6 address (no port can be told apart without brackets)
        if (!GoodV6(s)) {
            *err = "bad_host";
            return false;
        }
        *host = s;
        *v6 = true;
        return true;
    }
    size_t c = s.find(':');
    if (c == std::string::npos) {
        *host = s;
    } else {
        *host = s.substr(0, c);
        if (!ParsePort(s.substr(c + 1), port)) {
            *err = "bad_port";
            return false;
        }
    }
    if (!GoodHostChars(*host)) {
        *err = "bad_host";
        return false;
    }
    return true;
}

const char* Address::KindName(Kind k) {
    switch (k) {
        case Kind::Tcp:
            return "tcp";
        case Kind::Ws:
            return "ws";
        case Kind::Wss:
            return "wss";
    }
    return "?";
}

static std::string HostPart(const std::string& host) {
    return host.find(':') != std::string::npos ? "[" + host + "]" : host;
}

std::string Address::Url() const {
    if (kind == Kind::Tcp) {
        return "ws://" + HostPart(host) + ":" + std::to_string(port) + "/";
    }
    bool standard = (kind == Kind::Ws && port == 80) || (kind == Kind::Wss && port == 443);
    return std::string(kind == Kind::Ws ? "ws://" : "wss://") + HostPart(host) +
           (standard ? "" : ":" + std::to_string(port)) + path;
}

std::string Address::Text() const {
    if (kind == Kind::Tcp) {
        return HostPart(host) + ":" + std::to_string(port);
    }
    return Url();
}

Address ParseAddress(const std::string& textIn, uint16_t defaultPort) {
    Address a;
    std::string text = Trim(textIn);
    if (text.empty()) {
        a.error = "empty";
        return a;
    }
    if (std::any_of(text.begin(), text.end(), [](unsigned char c) { return std::isspace(c); })) {
        a.error = "bad_host";
        return a;
    }
    std::string low = Lower(text);
    size_t scheme = low.find("://");
    if (scheme != std::string::npos) {
        std::string s = low.substr(0, scheme);
        if (s == "ws" || s == "http") {
            a.kind = Address::Kind::Ws;
        } else if (s == "wss" || s == "https") {
            a.kind = Address::Kind::Wss;
        } else {
            a.error = "bad_scheme";
            return a;
        }
        std::string rest = text.substr(scheme + 3);
        size_t slash = rest.find_first_of("/?#");
        std::string hostport = slash == std::string::npos ? rest : rest.substr(0, slash);
        std::string path = slash == std::string::npos ? "/" : rest.substr(slash);
        if (!path.empty() && path[0] != '/') {
            path = "/" + path; // ("?x" or "#x" right after the host)
        }
        size_t hash = path.find('#');
        if (hash != std::string::npos) {
            path = path.substr(0, hash);
        }
        bool v6;
        uint16_t port;
        if (!SplitHostPort(hostport, &a.host, &port, &v6, &a.error)) {
            return a;
        }
        a.port = port != 0 ? port : (a.kind == Address::Kind::Ws ? 80 : 443);
        a.path = path.empty() ? "/" : path;
        a.ok = true;
        return a;
    }
    bool v6;
    uint16_t port;
    if (!SplitHostPort(text, &a.host, &port, &v6, &a.error)) {
        return a;
    }
    if (port == 443) {
        a.kind = Address::Kind::Wss;
        a.port = 443;
        a.path = "/";
        a.ok = true;
        return a;
    }
    a.kind = Address::Kind::Tcp;
    a.port = port != 0 ? port : defaultPort;
    a.ok = a.port != 0;
    if (!a.ok) {
        a.error = "bad_port";
    }
    bool hasLetter = std::any_of(a.host.begin(), a.host.end(), [](unsigned char c) { return std::isalpha(c); });
    a.bareDomain = a.ok && port == 0 && !v6 && !IsIPv4(a.host) && hasLetter && Lower(a.host) != "localhost" &&
                   a.host.find('.') != std::string::npos;
    return a;
}

Address WssFallback(const Address& a) {
    Address w = a;
    w.kind = Address::Kind::Wss;
    w.port = 443;
    w.path = "/";
    w.bareDomain = false;
    return w;
}

} // namespace Zmp
