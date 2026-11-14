#include "Lan.h"

#ifdef __EMSCRIPTEN__

namespace Zmp::Lan {
std::vector<Room> Rooms() {
    return {};
}
void StopListening() {
}
bool Listening() {
    return false;
}
void SetAnnounce(const std::string&, uint16_t, const std::string&, int, bool, const std::string&) {
}
void StopAnnounce() {
}
void Shutdown() {
}
} // namespace Zmp::Lan

#else

#include <atomic>
#include <chrono>
#include <map>
#include <mutex>
#include <thread>

#include <nlohmann/json.hpp>
#include <libultraship/bridge/consolevariablebridge.h>

#include "soh/Zmp/ZmpCVars.h"
#include "soh/Zmp/ZmpLog.h"

#ifdef _WIN32
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#pragma comment(lib, "ws2_32.lib")
typedef SOCKET SockT;
#define ZMP_BAD_SOCK INVALID_SOCKET
#define ZmpCloseSock closesocket
#else
#include <arpa/inet.h>
#include <netinet/in.h>
#include <sys/select.h>
#include <sys/socket.h>
#include <unistd.h>
typedef int SockT;
#define ZMP_BAD_SOCK (-1)
#define ZmpCloseSock close
#endif

namespace Zmp::Lan {

namespace {

constexpr const char* kGroup = "239.255.47.100";
using Clock = std::chrono::steady_clock;

struct Seen {
    Room room;
    Clock::time_point at;
};

struct State {
    std::mutex mutex;
    // listening
    std::thread listener;
    std::atomic<bool> listenStop{ false };
    bool listening = false;
    std::map<std::string, Seen> seen; // key: address + "\n" + room
    // announcing
    std::thread announcer;
    std::atomic<bool> announceStop{ false };
    bool announcing = false;
    std::string payload;
};

State& S() {
    static State* s = new State(); // (never destroyed: threads may outlive the statics)
    return *s;
}

void InitSockets() {
#ifdef _WIN32
    static bool sDone = [] {
        WSADATA d;
        return WSAStartup(MAKEWORD(2, 2), &d) == 0;
    }();
    (void)sDone;
#endif
}

uint16_t Port() {
    int p = CVarGetInteger(ZMP_CVAR_LAN_PORT, 47100);
    return (uint16_t)(p > 0 && p < 65536 ? p : 47100);
}

std::string TestAddress() {
    return CVarGetString(ZMP_CVAR_LAN_ADDRESS, "");
}

void ListenLoop(uint16_t port, std::string bindAddr) {
    InitSockets();
    SockT s = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (s == ZMP_BAD_SOCK) {
        Log("lan: no UDP socket");
        return;
    }
    int one = 1;
    setsockopt(s, SOL_SOCKET, SO_REUSEADDR, (const char*)&one, sizeof(one));
    sockaddr_in local{};
    local.sin_family = AF_INET;
    local.sin_port = htons(port);
    local.sin_addr.s_addr = htonl(INADDR_ANY);
    if (!bindAddr.empty()) {
        inet_pton(AF_INET, bindAddr.c_str(), &local.sin_addr);
    }
    if (bind(s, (sockaddr*)&local, sizeof(local)) != 0) {
        Log("lan: cannot listen on UDP " + (bindAddr.empty() ? std::string("*") : bindAddr) + ":" +
            std::to_string(port));
        ZmpCloseSock(s);
        return;
    }
    if (bindAddr.empty()) {
        ip_mreq m{};
        inet_pton(AF_INET, kGroup, &m.imr_multiaddr);
        m.imr_interface.s_addr = htonl(INADDR_ANY);
        setsockopt(s, IPPROTO_IP, IP_ADD_MEMBERSHIP, (const char*)&m, sizeof(m));
    }
    Log("lan: listening for games on UDP " + (bindAddr.empty() ? std::string("*") : bindAddr) + ":" +
        std::to_string(port));
    char buf[8192];
    while (!S().listenStop) {
        fd_set rd;
        FD_ZERO(&rd);
        FD_SET(s, &rd);
        timeval tv{ 0, 250000 };
        if (select((int)s + 1, &rd, nullptr, nullptr, &tv) <= 0) {
            continue;
        }
        sockaddr_in from{};
        socklen_t flen = sizeof(from);
        int n = recvfrom(s, buf, sizeof(buf), 0, (sockaddr*)&from, &flen);
        if (n <= 0) {
            continue;
        }
        char ip[64] = {};
        inet_ntop(AF_INET, &from.sin_addr, ip, sizeof(ip));
        try {
            nlohmann::json j = nlohmann::json::parse(std::string(buf, (size_t)n));
            if (j.value("zmp", 0) != 1 || !j.contains("rooms") || !j["rooms"].is_array()) {
                continue;
            }
            int p = j.value("port", 47100);
            if (p <= 0 || p > 65535) {
                continue;
            }
            std::lock_guard<std::mutex> lk(S().mutex);
            for (auto& r : j["rooms"]) {
                Room room;
                room.address = std::string(ip) + ":" + std::to_string(p);
                room.serverName = j.value("server_name", std::string()).substr(0, 64);
                room.room = r.value("name", std::string()).substr(0, 64);
                room.players = r.value("players", 0);
                room.max = r.value("max", 6);
                room.proto = j.value("proto", 0);
                room.engine = r.value("engine", std::string());
                room.buildHash = r.value("build_hash", std::string());
                room.password = r.value("password", false);
                if (room.room.empty()) {
                    continue;
                }
                S().seen[room.address + "\n" + room.room] = { room, Clock::now() };
            }
        } catch (const std::exception&) {
            // (not ours: ignored)
        }
    }
    ZmpCloseSock(s);
}

void AnnounceLoop(uint16_t port, std::string target) {
    InitSockets();
    SockT s = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (s == ZMP_BAD_SOCK) {
        return;
    }
    int one = 1;
    setsockopt(s, SOL_SOCKET, SO_BROADCAST, (const char*)&one, sizeof(one));
    unsigned char ttl = 1; // (never beyond this network)
    setsockopt(s, IPPROTO_IP, IP_MULTICAST_TTL, (const char*)&ttl, sizeof(ttl));
    unsigned char loop = 1; // (a list on this same PC sees it too)
    setsockopt(s, IPPROTO_IP, IP_MULTICAST_LOOP, (const char*)&loop, sizeof(loop));
    std::vector<sockaddr_in> to;
    auto add = [&](const char* ip) {
        sockaddr_in a{};
        a.sin_family = AF_INET;
        a.sin_port = htons(port);
        inet_pton(AF_INET, ip, &a.sin_addr);
        to.push_back(a);
    };
    if (!target.empty()) {
        add(target.c_str());
    } else {
        add(kGroup);
        add("255.255.255.255");
    }
    Log("lan: announcing on UDP " + (target.empty() ? std::string(kGroup) + " and broadcast" : target) + ":" +
        std::to_string(port));
    while (!S().announceStop) {
        std::string payload;
        {
            std::lock_guard<std::mutex> lk(S().mutex);
            payload = S().payload;
        }
        for (auto& a : to) {
            sendto(s, payload.data(), (int)payload.size(), 0, (sockaddr*)&a, sizeof(a));
        }
        for (int i = 0; i < 20 && !S().announceStop; i++) {
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }
    }
    ZmpCloseSock(s);
}

} // namespace

std::vector<Room> Rooms() {
    State& st = S();
    if (!st.listening) {
        st.listenStop = false;
        st.listening = true;
        st.listener = std::thread(ListenLoop, Port(), TestAddress());
    }
    std::vector<Room> out;
    std::lock_guard<std::mutex> lk(st.mutex);
    auto now = Clock::now();
    for (auto it = st.seen.begin(); it != st.seen.end();) {
        double age = std::chrono::duration<double>(now - it->second.at).count();
        if (age > 6.0) {
            it = st.seen.erase(it);
            continue;
        }
        Room r = it->second.room;
        r.ageSeconds = age;
        out.push_back(r);
        ++it;
    }
    return out;
}

bool Listening() {
    return S().listening;
}

void StopListening() {
    State& st = S();
    if (!st.listening) {
        return;
    }
    st.listenStop = true;
    if (st.listener.joinable()) {
        st.listener.join();
    }
    st.listening = false;
    std::lock_guard<std::mutex> lk(st.mutex);
    st.seen.clear();
}

void SetAnnounce(const std::string& serverName, uint16_t port, const std::string& room, int players, bool password,
                 const std::string& buildHash) {
    State& st = S();
    nlohmann::json j = { { "zmp", 1 },
                         { "server_name", serverName },
                         { "port", port },
                         { "proto", 5 },
                         { "rooms", nlohmann::json::array({ { { "name", room },
                                                              { "players", players },
                                                              { "max", 6 },
                                                              { "engine", "native" },
                                                              { "build_hash", buildHash },
                                                              { "password", password } } }) } };
    {
        std::lock_guard<std::mutex> lk(st.mutex);
        st.payload = j.dump();
    }
    if (!st.announcing) {
        st.announceStop = false;
        st.announcing = true;
        st.announcer = std::thread(AnnounceLoop, Port(), TestAddress());
    }
}

void StopAnnounce() {
    State& st = S();
    if (!st.announcing) {
        return;
    }
    st.announceStop = true;
    if (st.announcer.joinable()) {
        st.announcer.join();
    }
    st.announcing = false;
}

void Shutdown() {
    StopAnnounce();
    StopListening();
}

} // namespace Zmp::Lan

#endif
