#include "ZmpClient.h"

#include <chrono>
#include <condition_variable>
#include <deque>
#include <thread>
#include <filesystem>

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#include <emscripten/websocket.h>
#else
#include <SDL2/SDL_net.h>
#endif
#include <nlohmann/json.hpp>
#include <ship/Context.h>
#include <libultraship/bridge/consolevariablebridge.h>

#include "ZmpProtocol.h"
#include "Lockstep.h"
#include "LogShip.h"
#include "soh/Zmp/Sim/CVarProfile.h"
#include "soh/Zmp/State/ResourceSlots.h"
#include "soh/Zmp/ZmpCVars.h"
#include "soh/Zmp/ZmpLog.h"
#include "soh/GameVersions.h"

extern "C" uint32_t ResourceMgr_GetNumGameVersions();
extern "C" uint32_t ResourceMgr_GetGameVersion(int index);

#ifdef _WIN32
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#else
#include <unistd.h>
#endif

namespace Zmp {

#ifdef __EMSCRIPTEN__
namespace {
// The one connection of the web client (D-239); every field is touched on the main thread only.
struct WebConn {
    EMSCRIPTEN_WEBSOCKET_T ws = 0;
    int generation = 0; // callbacks of an older socket are ignored
    bool open = false;  // HELLO sent, frames may go
    bool welcomed = false;
    bool rejected = false;
    double retryAt = 0; // emscripten_get_now() ms; 0 = no retry pending
    double lastPing = 0;
    uint64_t pingNonce = 0;
    std::string host;
    uint16_t port = 0;
    std::string room;
    std::string name;
};
WebConn sWeb;
} // namespace
#endif

static std::string ExecutablePath() {
#ifdef _WIN32
    wchar_t buf[4096];
    DWORD len = GetModuleFileNameW(NULL, buf, 4096);
    if (len == 0 || len >= 4096) {
        return "";
    }
    return std::filesystem::path(std::wstring(buf, len)).string();
#else
    char buf[4096];
    ssize_t len = readlink("/proc/self/exe", buf, sizeof(buf) - 1);
    if (len <= 0) {
        return "";
    }
    return std::string(buf, (size_t)len);
#endif
}

Client& Client::Get() {
    static Client sInstance;
    return sInstance;
}

const char* Client::StateName(NetState state) {
    switch (state) {
        case NetState::Disconnected:
            return "disconnected";
        case NetState::Connecting:
            return "connecting";
        case NetState::Connected:
            return "connected";
        case NetState::Rejected:
            return "rejected";
        case NetState::Error:
            return "error";
    }
    return "unknown";
}

std::string RomVersionName(uint32_t crc) {
    switch (crc) {
        case OOT_NTSC_US_10:
            return "NTSC N64 1.0";
        case OOT_NTSC_US_11:
            return "NTSC N64 1.1";
        case OOT_NTSC_US_12:
            return "NTSC N64 1.2";
        case OOT_PAL_10:
            return "PAL N64 1.0";
        case OOT_PAL_11:
            return "PAL N64 1.1";
        case OOT_NTSC_JP_GC_CE:
            return "NTSC GameCube Japon (Collector's Edition)";
        case OOT_NTSC_JP_GC:
            return "NTSC GameCube Japon";
        case OOT_NTSC_US_GC:
            return "NTSC GameCube USA";
        case OOT_PAL_GC:
            return "PAL GameCube";
        case OOT_NTSC_JP_MQ:
            return "NTSC Master Quest Japon";
        case OOT_NTSC_US_MQ:
            return "NTSC Master Quest USA";
        case OOT_PAL_MQ:
            return "PAL Master Quest";
        case OOT_PAL_GC_DBG1:
        case OOT_PAL_GC_DBG2:
            return "PAL GameCube Debug";
        case OOT_PAL_GC_MQ_DBG:
            return "PAL Master Quest Debug";
        case OOT_IQUE_TW:
            return "iQue Taiwan";
        case OOT_IQUE_CN:
            return "iQue China";
    }
    char t[40];
    snprintf(t, sizeof(t), "version desconocida (%08X)", crc);
    return t;
}

Handshake Client::GetHandshake() {
    std::lock_guard<std::mutex> lock(mHandshakeMutex);
    if (!mHandshakeReady) {
#ifdef __EMSCRIPTEN__
        // Web (D-241): the page hashed soh.wasm (zmp-web.js); same wasm, same identity.
        mHandshake.buildHash = emscripten_run_script_string("(Module.zmp && Module.zmp.buildHash) || 'missing'");
#else
        mHandshake.buildHash = HashFile(ExecutablePath());
#endif
        // Content hash of the simulation resources: two extractions of the same ROM match even though the
        // zip files differ (reports/fase2/ROMS.md), plus the list of resource paths (save states need it).
        uint32_t simEntries = 0;
        std::string sim = HashAssetsSim(Ship::Context::LocateFileAcrossAppDirs("oot.o2r"), &simEntries);
        char files[24];
        snprintf(files, sizeof(files), "%016llx", (unsigned long long)ResSlots::FileListHash());
        mHandshake.ootHash = sim + "-" + files;
        mHandshake.sohHash = HashFile(Ship::Context::LocateFileAcrossAppDirs("soh.o2r"));
        char prof[24];
        snprintf(prof, sizeof(prof), "%016llx", (unsigned long long)CVarProfile::Hash());
        mHandshake.cvarProfileHash = prof;
        Log("handshake: oot sim entries=" + std::to_string(simEntries));
        mHandshake.randoSeed = 0;
        if (ResourceMgr_GetNumGameVersions() > 0) {
            mHandshake.romName = RomVersionName(ResourceMgr_GetGameVersion(0));
        }
        mHandshakeReady = true;
        Log("handshake hashes: build=" + mHandshake.buildHash + " oot=" + mHandshake.ootHash +
            " soh=" + mHandshake.sohHash);
    }
    Handshake hs = mHandshake;
#ifdef ZMP_HARNESS
    const char* fake = CVarGetString(ZMP_CVAR_DEBUG_BUILD_HASH, "");
    if (fake != nullptr && fake[0] != '\0') {
        hs.buildHash = fake;
    }
    const char* rom = CVarGetString(ZMP_CVAR_DEBUG_ROM, "");
    if (rom != nullptr && rom[0] != '\0') {
        hs.romName = RomVersionName((uint32_t)strtoul(rom, nullptr, 16));
        hs.ootHash += std::string("-") + rom;
    }
#endif
    return hs;
}

void Client::SetState(NetState state, const std::string& reason, const std::string& detail) {
    std::lock_guard<std::mutex> lock(mMutex);
    mStatus.state = state;
    mStatus.reason = reason;
    mStatus.detail = detail;
    if (state != NetState::Connected) {
        mStatus.players.clear();
        mStatus.rttMs = -1;
    }
}

NetStatus Client::GetStatus() {
    std::lock_guard<std::mutex> lock(mMutex);
    return mStatus;
}

void Client::Connect(const std::string& host, uint16_t port, const std::string& room, const std::string& name) {
    Disconnect();
    {
        std::lock_guard<std::mutex> lock(mMutex);
        mStatus = NetStatus();
        mStatus.state = NetState::Connecting;
        mStatus.host = host;
        mStatus.port = port;
        mStatus.room = room;
        mStatus.name = name;
    }
    mStop = false;
#ifdef __EMSCRIPTEN__
    sWeb.host = host;
    sWeb.port = port;
    sWeb.room = room;
    sWeb.name = name;
    WebStart();
#else
    mThread = std::thread(&Client::Run, this, host, port, room, name);
#endif
}

void Client::Disconnect() {
    mStop = true;
#ifdef __EMSCRIPTEN__
    sWeb.retryAt = 0;
    if (sWeb.ws != 0) {
        emscripten_websocket_close(sWeb.ws, 1000, "bye");
        emscripten_websocket_delete(sWeb.ws);
        sWeb.ws = 0;
        sWeb.generation++;
    }
    sWeb.open = false;
    if (sWeb.welcomed) {
        sWeb.welcomed = false;
        Lockstep::OnDisconnected();
        LogShip::OnSessionEnd();
    }
#else
    if (mThread.joinable()) {
        mThread.join();
    }
#endif
    std::lock_guard<std::mutex> lock(mMutex);
    if (mStatus.state == NetState::Connecting || mStatus.state == NetState::Connected ||
        mStatus.state == NetState::Error) {
        mStatus.state = NetState::Disconnected;
        mStatus.players.clear();
        mStatus.rttMs = -1;
    }
}

#ifndef __EMSCRIPTEN__
static bool SendAll(TCPsocket sock, const std::vector<uint8_t>& data) {
    return SDLNet_TCP_Send(sock, data.data(), (int)data.size()) == (int)data.size();
}
#endif

// Phase 5b: nothing the game thread sends may hold it (the state of a group for a player who enters is 50 KB; on a
// real connection the socket takes its time). Messages are queued and one writer thread puts them on the socket in
// order. The queue lives for the whole process (never destroyed: the writer thread may outlive the statics).
#ifndef __EMSCRIPTEN__
namespace {
struct Outbox {
    std::mutex mutex;
    std::condition_variable cv;
    std::deque<std::vector<uint8_t>> frames;
    bool open = false; // a connection past its HELLO exists
    bool started = false;
};
Outbox* sOut = new Outbox();
} // namespace
#endif

bool Client::Send(const nlohmann::json& msg) {
#ifdef __EMSCRIPTEN__
    // Web (D-239): one binary WebSocket message per body, no length prefix; sent at once (one thread).
    if (!sWeb.open || sWeb.ws == 0) {
        return false;
    }
    std::vector<uint8_t> body = nlohmann::json::to_msgpack(msg);
    return emscripten_websocket_send_binary(sWeb.ws, body.data(), (uint32_t)body.size()) == EMSCRIPTEN_RESULT_SUCCESS;
#else
    std::vector<uint8_t> frame = EncodeFrame(msg);
    {
        std::lock_guard<std::mutex> lock(sOut->mutex);
        if (!sOut->open) {
            return false;
        }
        sOut->frames.push_back(std::move(frame));
        if (!sOut->started) {
            sOut->started = true;
            std::thread([this]() {
                for (;;) {
                    std::vector<uint8_t> next;
                    {
                        std::unique_lock<std::mutex> lk(sOut->mutex);
                        sOut->cv.wait(lk, [] { return !sOut->frames.empty(); });
                        next = std::move(sOut->frames.front());
                        sOut->frames.pop_front();
                    }
                    std::lock_guard<std::mutex> lk(mSendMutex);
                    auto t0 = std::chrono::steady_clock::now();
                    if (mSock != nullptr && !SendAll((TCPsocket)mSock, next)) {
                        Log("net: send failed (" + std::to_string(next.size()) + " bytes)");
                    }
                    double ms =
                        std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - t0).count();
                    if (ms > 20.0) {
                        Log("net: slow send, " + std::to_string(next.size()) + " bytes took " +
                            std::to_string((int)ms) + " ms on the socket");
                    }
                }
            }).detach();
        }
    }
    sOut->cv.notify_one();
    return true;
#endif
}

static uint64_t SteadyMs() {
    return (uint64_t)std::chrono::duration_cast<std::chrono::milliseconds>(
               std::chrono::steady_clock::now().time_since_epoch())
        .count();
}

static void ReadPlayers(const nlohmann::json& msg, std::vector<NetPlayer>& out) {
    out.clear();
    if (!msg.contains("players") || !msg["players"].is_array()) {
        return;
    }
    for (auto& p : msg["players"]) {
        NetPlayer np;
        np.id = p.value("id", 0u);
        np.name = p.value("name", std::string());
        np.slot = p.value("slot", -1);
        np.rtt = p.value("rtt", -1);
        np.state = p.value("state", std::string());
        np.leader = p.value("leader", false);
        out.push_back(np);
    }
}

bool Client::HandleMessage(nlohmann::json& msg, const std::string& room, bool& welcomed, bool& rejected) {
    bool ok = true;
    std::string t = msg.value("t", std::string());
    if (t == "WELCOME") {
        std::lock_guard<std::mutex> lock(mMutex);
        mStatus.state = NetState::Connected;
        mStatus.reason.clear();
        mStatus.detail.clear();
        mStatus.playerId = msg.value("player_id", 0u);
        mStatus.room = msg.value("room", room);
        ReadPlayers(msg, mStatus.players);
        Log("net: WELCOME player_id=" + std::to_string(mStatus.playerId) + " room=" + mStatus.room +
            (msg.value("rejoin", false) ? " (rejoin)" : ""));
        welcomed = true;
        LogShip::OnWelcome(msg.value("log_collect", false), msg.value("log_token", std::string()), mStatus.host,
                           mStatus.port);
        Lockstep::OnConnected(msg.value("rejoin", false), msg.value("leave_shared", false));
    } else if (t == "REJECT") {
        std::string reason = msg.value("reason", std::string("rejected"));
        std::string detail = msg.value("detail", std::string());
        SetState(NetState::Rejected, reason, detail);
        Log("net: REJECT reason=" + reason + " detail=" + detail);
        rejected = true;
    } else if (t == "PLAYER_LIST") {
        {
            std::lock_guard<std::mutex> lock(mMutex);
            ReadPlayers(msg, mStatus.players);
        }
        Lockstep::OnNetMessage(std::move(msg));
    } else if (t == "PONG") {
        uint64_t sentAt = msg.value("sent_at", (uint64_t)0);
        std::lock_guard<std::mutex> lock(mMutex);
        mStatus.rttMs = (int)(SteadyMs() - sentAt);
    } else if (t == "PING") {
        nlohmann::json pong = { { "t", "PONG" },
                                { "nonce", msg.value("nonce", (uint64_t)0) },
                                { "sent_at", msg.value("sent_at", (uint64_t)0) } };
        ok = Send(pong);
    } else if (t == "KICK") {
        SetState(NetState::Rejected, "kicked", msg.value("reason", std::string()));
        rejected = true;
    } else if (!t.empty()) {
        Lockstep::OnNetMessage(std::move(msg));
    }
    return ok;
}

#ifndef __EMSCRIPTEN__
void Client::Run(std::string host, uint16_t port, std::string room, std::string name) {
    Handshake hs = GetHandshake();

    while (!mStop) {
        SetState(NetState::Connecting);
        IPaddress addr;
        TCPsocket sock = nullptr;
        if (SDLNet_ResolveHost(&addr, host.c_str(), port) == 0) {
            sock = SDLNet_TCP_Open(&addr);
        }
        if (sock == nullptr) {
            SetState(NetState::Error, "unreachable", "No se pudo conectar a " + host + ":" + std::to_string(port));
            Log("net: connect failed to " + host + ":" + std::to_string(port));
            // Retry every 2 s until Disconnect() is called.
            for (int i = 0; i < 20 && !mStop; i++) {
                std::this_thread::sleep_for(std::chrono::milliseconds(100));
            }
            continue;
        }

        Log("net: tcp connected to " + host + ":" + std::to_string(port) + ", sending HELLO");
        nlohmann::json hello = {
            { "t", "HELLO" },
            { "proto", kProtocolVersion },
            { "build_hash", hs.buildHash },
            { "oot_hash", hs.ootHash },
            { "soh_hash", hs.sohHash },
            { "cvar_profile_hash", hs.cvarProfileHash },
            { "rando_seed", hs.randoSeed },
            { "rom_name", hs.romName },
            { "name", name },
            { "room", room },
            { "client_nonce", SteadyMs() },
        };
        SDLNet_SocketSet set = SDLNet_AllocSocketSet(1);
        SDLNet_TCP_AddSocket(set, sock);
        bool ok;
        {
            std::lock_guard<std::mutex> lock(mSendMutex);
            mSock = sock;
            ok = SendAll(sock, EncodeFrame(hello));
        }
        {
            std::lock_guard<std::mutex> lock(sOut->mutex);
            sOut->frames.clear();
            sOut->open = ok;
        }
        bool welcomed = false;
        FrameReader reader;
        uint64_t lastPing = 0;
        uint64_t pingNonce = 0;
        bool rejected = false;

        while (ok && !mStop) {
            int ready = SDLNet_CheckSockets(set, 100);
            if (ready < 0) {
                ok = false;
                break;
            }
            if (ready > 0 && SDLNet_SocketReady(sock)) {
                uint8_t buf[4096];
                int n = SDLNet_TCP_Recv(sock, buf, sizeof(buf));
                if (n <= 0) {
                    ok = false;
                    break;
                }
                reader.Append(buf, (size_t)n);
                nlohmann::json msg;
                try {
                    while (reader.Next(msg)) {
                        if (!HandleMessage(msg, room, welcomed, rejected)) {
                            ok = false;
                        }
                    }
                } catch (const std::exception& e) {
                    Log(std::string("net: protocol error: ") + e.what());
                    ok = false;
                }
                if (rejected) {
                    break;
                }
            }
            uint64_t now = SteadyMs();
            if (ok && GetStatus().state == NetState::Connected && now - lastPing >= 1000) {
                lastPing = now;
                nlohmann::json ping = { { "t", "PING" }, { "nonce", ++pingNonce }, { "sent_at", now } };
                ok = Send(ping);
            }
        }

        // (what was already handed over still goes out, e.g. the LEAVE_GROUP of a player who quits)
        for (int i = 0; i < 20; i++) {
            {
                std::lock_guard<std::mutex> lock(sOut->mutex);
                if (sOut->frames.empty()) {
                    break;
                }
            }
            std::this_thread::sleep_for(std::chrono::milliseconds(10));
        }
        {
            std::lock_guard<std::mutex> lock(sOut->mutex);
            sOut->open = false;
            sOut->frames.clear();
        }
        {
            std::lock_guard<std::mutex> lock(mSendMutex);
            mSock = nullptr;
        }
        if (welcomed) {
            Lockstep::OnDisconnected();
            LogShip::OnSessionEnd();
        }
        SDLNet_TCP_DelSocket(set, sock);
        SDLNet_FreeSocketSet(set);
        SDLNet_TCP_Close(sock);

        if (rejected || mStop) {
            break;
        }
        SetState(NetState::Error, "connection_lost", "Conexion perdida con " + host);
        Log("net: connection lost, retrying");
        for (int i = 0; i < 20 && !mStop; i++) {
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }
    }
    if (mStop) {
        SetState(NetState::Disconnected);
    }
}

#endif // !__EMSCRIPTEN__

#ifdef __EMSCRIPTEN__
// ---- Web transport (D-239) ----------------------------------------------------------------------------------------
// The URL of the server: "ws://host:port/" (port 443: "wss://host/"); a host that already is a ws:// or wss:// URL is
// used as it is (what the player types in the page for a TLS server).
static std::string WebUrl(const std::string& host, uint16_t port) {
    if (host.rfind("ws://", 0) == 0 || host.rfind("wss://", 0) == 0) {
        return host;
    }
    if (port == 443) {
        return "wss://" + host + "/";
    }
    return "ws://" + host + ":" + std::to_string(port) + "/";
}

static bool WebCurrent(void* userData) {
    return (int)(intptr_t)userData == sWeb.generation && sWeb.ws != 0;
}

static EM_BOOL WebOnOpen(int, const EmscriptenWebSocketOpenEvent*, void* userData) {
    if (WebCurrent(userData)) {
        Client::Get().WebOpen();
    }
    return EM_TRUE;
}

static EM_BOOL WebOnMessage(int, const EmscriptenWebSocketMessageEvent* e, void* userData) {
    if (!WebCurrent(userData) || e->isText) {
        return EM_TRUE;
    }
    try {
        nlohmann::json msg = nlohmann::json::from_msgpack(e->data, e->data + e->numBytes);
        Client::Get().WebMessage(msg);
    } catch (const std::exception& ex) {
        Log(std::string("net: protocol error: ") + ex.what());
        Client::Get().WebClosed(true);
    }
    return EM_TRUE;
}

static EM_BOOL WebOnError(int, const EmscriptenWebSocketErrorEvent*, void* userData) {
    if (WebCurrent(userData)) {
        Client::Get().WebClosed(true);
    }
    return EM_TRUE;
}

static EM_BOOL WebOnClose(int, const EmscriptenWebSocketCloseEvent* e, void* userData) {
    if (WebCurrent(userData)) {
        Log("net: websocket closed code=" + std::to_string(e->code) + " clean=" + (e->wasClean ? "yes" : "no"));
        Client::Get().WebClosed(false);
    }
    return EM_TRUE;
}

void Client::WebStart() {
    SetState(NetState::Connecting);
    sWeb.open = false;
    sWeb.welcomed = false;
    sWeb.rejected = false;
    sWeb.retryAt = 0;
    if (!emscripten_websocket_is_supported()) {
        SetState(NetState::Error, "unsupported", "Este navegador no tiene WebSocket");
        return;
    }
    std::string url = WebUrl(sWeb.host, sWeb.port);
    EmscriptenWebSocketCreateAttributes attr;
    emscripten_websocket_init_create_attributes(&attr);
    attr.url = url.c_str();
    attr.protocols = nullptr;
    attr.createOnMainThread = EM_TRUE;
    EMSCRIPTEN_WEBSOCKET_T ws = emscripten_websocket_new(&attr);
    if (ws <= 0) {
        SetState(NetState::Error, "unreachable", "No se pudo abrir " + url);
        Log("net: websocket open failed for " + url + " (" + std::to_string(ws) + ")");
        sWeb.retryAt = emscripten_get_now() + 2000.0;
        return;
    }
    sWeb.ws = ws;
    sWeb.generation++;
    void* ud = (void*)(intptr_t)sWeb.generation;
    emscripten_websocket_set_onopen_callback(ws, ud, WebOnOpen);
    emscripten_websocket_set_onmessage_callback(ws, ud, WebOnMessage);
    emscripten_websocket_set_onerror_callback(ws, ud, WebOnError);
    emscripten_websocket_set_onclose_callback(ws, ud, WebOnClose);
    Log("net: websocket opening " + url);
}

void Client::WebOpen() {
    Handshake hs = GetHandshake();
    nlohmann::json hello = {
        { "t", "HELLO" },
        { "proto", kProtocolVersion },
        { "build_hash", hs.buildHash },
        { "oot_hash", hs.ootHash },
        { "soh_hash", hs.sohHash },
        { "cvar_profile_hash", hs.cvarProfileHash },
        { "rando_seed", hs.randoSeed },
        { "rom_name", hs.romName },
        { "name", sWeb.name },
        { "room", sWeb.room },
        { "client_nonce", SteadyMs() },
    };
    sWeb.open = true;
    sWeb.lastPing = emscripten_get_now();
    if (!Send(hello)) {
        Log("net: websocket HELLO send failed");
        WebClosed(true);
        return;
    }
    Log("net: websocket connected to " + WebUrl(sWeb.host, sWeb.port) + ", HELLO sent");
}

void Client::WebMessage(nlohmann::json& msg) {
    bool ok = HandleMessage(msg, sWeb.room, sWeb.welcomed, sWeb.rejected);
    if (sWeb.rejected || !ok) {
        // (the socket is closed; WebClosed runs from the close callback and, rejected, does not retry)
        if (sWeb.ws != 0) {
            emscripten_websocket_close(sWeb.ws, 1000, "");
        }
    }
}

void Client::WebClosed(bool error) {
    if (sWeb.ws != 0) {
        emscripten_websocket_delete(sWeb.ws);
        sWeb.ws = 0;
        sWeb.generation++;
    }
    bool wasOpen = sWeb.open;
    sWeb.open = false;
    if (sWeb.welcomed) {
        sWeb.welcomed = false;
        Lockstep::OnDisconnected();
        LogShip::OnSessionEnd();
    }
    if (mStop) {
        SetState(NetState::Disconnected);
        return;
    }
    if (sWeb.rejected) {
        return; // (state Rejected already set by HandleMessage)
    }
    std::string where = sWeb.host + ":" + std::to_string(sWeb.port);
    if (!wasOpen) {
        SetState(NetState::Error, "unreachable", "No se pudo conectar a " + where);
        Log("net: websocket connect failed to " + where + (error ? " (error)" : ""));
    } else {
        SetState(NetState::Error, "connection_lost", "Conexion perdida con " + sWeb.host);
        Log("net: websocket connection lost, retrying");
    }
    // Retry every 2 s until Disconnect() is called (same as the native thread).
    sWeb.retryAt = emscripten_get_now() + 2000.0;
}

void Client::PollWeb() {
    if (mStop) {
        return;
    }
    double now = emscripten_get_now();
    if (sWeb.ws == 0 && sWeb.retryAt > 0 && now >= sWeb.retryAt) {
        sWeb.retryAt = 0;
        WebStart();
        return;
    }
    if (sWeb.open && sWeb.welcomed && now - sWeb.lastPing >= 1000.0) {
        sWeb.lastPing = now;
        nlohmann::json ping = { { "t", "PING" }, { "nonce", ++sWeb.pingNonce }, { "sent_at", SteadyMs() } };
        Send(ping);
    }
}
#endif

} // namespace Zmp
