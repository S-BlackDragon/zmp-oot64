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
#include "Transport.h"
#endif
#include "Address.h"
#include "Crypto.h"
#include <nlohmann/json.hpp>
#include <ship/Context.h>
#include <ship/window/Window.h>
#include <libultraship/bridge/consolevariablebridge.h>

#include "ZmpProtocol.h"
#include "Lockstep.h"
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
    ConnectRequest req;
    size_t addrIndex = 0; // the address of req.addresses being tried
    std::string host;     // (that address)
    uint16_t port = 0;
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
    ConnectRequest req;
    req.addresses = { host };
    req.defaultPort = port;
    req.room = room;
    req.name = name;
    Connect(std::move(req));
}

std::string Client::PlayerKey() {
    std::string key = CVarGetString(ZMP_CVAR_PLAYER_KEY, "");
    Crypto::Bytes b;
    if (key.size() != 32 || !Crypto::FromHex(key, b)) {
        Crypto::Bytes r = Crypto::Random(16);
        key = Crypto::Hex(r.data(), r.size());
        CVarSetString(ZMP_CVAR_PLAYER_KEY, key.c_str());
        Ship::Context::GetRawInstance()->GetWindow()->GetGui()->SaveConsoleVariablesNextFrame();
        Log("net: new player key made");
    }
    return key;
}

void Client::Connect(ConnectRequest req) {
    Disconnect();
    if (req.addresses.empty()) {
        req.addresses.push_back("");
    }
    req.playerKey = PlayerKey();
    req.color = CVarGetInteger(ZMP_CVAR_COLOR, -1);
    {
        std::lock_guard<std::mutex> lock(mMutex);
        mStatus = NetStatus();
        mStatus.state = NetState::Connecting;
        mStatus.host = req.addresses[0];
        mStatus.port = req.defaultPort;
        mStatus.room = req.room;
        mStatus.name = req.name;
    }
    mStop = false;
    mProto = 5;
    mFallbackPending = false;
#ifdef __EMSCRIPTEN__
    sWeb.req = req;
    sWeb.addrIndex = 0;
    sWeb.host = req.addresses[0];
    sWeb.port = req.defaultPort;
    WebStart();
#else
    mThread = std::thread(&Client::Run, this, std::move(req));
#endif
}

bool Client::Rename(const std::string& name) {
    if (GetStatus().state != NetState::Connected || mProto < 5) {
        return false;
    }
    return Send({ { "t", "RENAME" }, { "name", name } });
}

bool Client::SetColor(int rgb) {
    if (GetStatus().state != NetState::Connected || mProto < 5) {
        return false;
    }
    return Send({ { "t", "SET_COLOR" }, { "color", rgb } });
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
                    if (mSock != nullptr && !((Conn*)mSock)->SendFrame(next)) {
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

// A binary field of a message (MessagePack bin; a str is taken as its bytes).
static std::vector<uint8_t> BytesOf(const nlohmann::json& v) {
    if (v.is_binary()) {
        return std::vector<uint8_t>(v.get_binary().begin(), v.get_binary().end());
    }
    if (v.is_string()) {
        std::string s = v.get<std::string>();
        return std::vector<uint8_t>(s.begin(), s.end());
    }
    return {};
}

static nlohmann::json Bin(const uint8_t* data, size_t size) {
    return nlohmann::json::binary(std::vector<std::uint8_t>(data, data + size));
}

// The HELLO of this client (both transports, both builds). Protocol 5 (docs/PROTOCOLO_5.md 2.1) adds a new
// auth_nonce per connection, whether a password was typed, the player key, the colour and the engine; against an
// older server it falls back to protocol 4 once (no password possible there).
nlohmann::json Client::MakeHello(const ConnectRequest& req) {
    Handshake hs = GetHandshake();
    nlohmann::json hello = {
        { "t", "HELLO" },
        { "proto", mProto },
        { "build_hash", hs.buildHash },
        { "oot_hash", hs.ootHash },
        { "soh_hash", hs.sohHash },
        { "cvar_profile_hash", hs.cvarProfileHash },
        { "rando_seed", hs.randoSeed },
        { "rom_name", hs.romName },
        { "name", req.name },
        { "room", req.room },
        { "client_nonce", SteadyMs() },
    };
    mServerNonce.clear();
    mSentRoomProof = false;
    if (mProto >= 5) {
        mAuthNonce = Crypto::Random(16);
        hello["auth_nonce"] = Bin(mAuthNonce.data(), mAuthNonce.size());
        hello["has_password"] = !Crypto::NormalizePassword(req.password).empty();
        hello["player_key"] = req.playerKey;
        hello["color"] = req.color;
#ifdef __EMSCRIPTEN__
        hello["engine"] = "wasm";
#else
        hello["engine"] = "native";
#endif
    }
    return hello;
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
        np.color = p.value("color", -1);
        out.push_back(np);
    }
}

bool Client::HandleMessage(nlohmann::json& msg, const ConnectRequest& req, bool& welcomed, bool& rejected) {
    bool ok = true;
    const std::string& room = req.room;
    std::string t = msg.value("t", std::string());
    if (t == "WELCOME" && mSentRoomProof && msg.contains("room_confirm")) {
        // (a server that does not know the room's key cannot make this: it is not the room the password is for)
        Crypto::Digest key;
        std::copy(mRoomKey.begin(), mRoomKey.end(), key.begin());
        Crypto::Digest want = Crypto::Confirm(key, "room", mServerNonce, mAuthNonce);
        if (BytesOf(msg["room_confirm"]) != std::vector<uint8_t>(want.begin(), want.end())) {
            SetState(NetState::Rejected, "bad_confirm", "The server could not prove it knows the room's password");
            Log("net: WELCOME with a room_confirm that does not match: leaving");
            rejected = true;
            return ok;
        }
    }
    if (t == "CHALLENGE") {
        // docs/PROTOCOLO_5.md 2.3-2.4: the proofs this server asks for; a missing one is the server's to refuse.
        mServerNonce = BytesOf(msg["nonce"]);
        std::string pw = Crypto::NormalizePassword(req.password);
        nlohmann::json auth = { { "t", "AUTH" } };
        if ((msg.value("room", false) || msg.value("create", false)) && !pw.empty()) {
            std::string cacheKey = room + '\0' + pw;
            if (mKeyFor != cacheKey || mRoomKey.size() != 32) {
                Crypto::Digest k = Crypto::RoomKey(room, pw);
                mRoomKey.assign(k.begin(), k.end());
                mKeyFor = cacheKey;
            }
            Crypto::Digest key;
            std::copy(mRoomKey.begin(), mRoomKey.end(), key.begin());
            if (msg.value("room", false)) {
                Crypto::Digest proof = Crypto::Proof(key, "room", mServerNonce, mAuthNonce, room, req.name);
                auth["room_proof"] = Bin(proof.data(), proof.size());
                mSentRoomProof = true;
            }
            if (msg.value("create", false)) {
                auth["room_key"] = Bin(key.data(), key.size());
            }
        }
        if (msg.value("server", false) && !Crypto::NormalizePassword(req.serverPassword).empty()) {
            Crypto::Digest skey = Crypto::ServerKey(req.serverPassword);
            Crypto::Digest proof = Crypto::Proof(skey, "server", mServerNonce, mAuthNonce, room, req.name);
            auth["server_proof"] = Bin(proof.data(), proof.size());
        }
        Log(std::string("net: CHALLENGE (room=") + (msg.value("room", false) ? "yes" : "no") +
            " create=" + (msg.value("create", false) ? "yes" : "no") +
            " server=" + (msg.value("server", false) ? "yes" : "no") + "), AUTH sent");
        ok = Send(auth);
    } else if (t == "RENAME_RESULT") {
        std::lock_guard<std::mutex> lock(mMutex);
        bool good = msg.value("ok", false);
        if (good) {
            mStatus.name = msg.value("name", mStatus.name);
        }
        mStatus.renameResult = good ? "ok" : msg.value("reason", std::string("rejected"));
        mStatus.renameSeq++;
        Log("net: RENAME_RESULT " + mStatus.renameResult + " name=" + msg.value("name", std::string()));
    } else if (t == "WELCOME") {
        std::lock_guard<std::mutex> lock(mMutex);
        mStatus.proto = mProto;
        mStatus.roomPassword = msg.value("room_password", false);
        mStatus.roomOwner = msg.value("room_owner", false);
        mStatus.serverName = msg.value("server_name", std::string());
        mStatus.state = NetState::Connected;
        mStatus.reason.clear();
        mStatus.detail.clear();
        mStatus.playerId = msg.value("player_id", 0u);
        mStatus.room = msg.value("room", room);
        ReadPlayers(msg, mStatus.players);
        Log("net: WELCOME player_id=" + std::to_string(mStatus.playerId) + " room=" + mStatus.room +
            (msg.value("rejoin", false) ? " (rejoin)" : ""));
        welcomed = true;
        Lockstep::OnConnected(msg.value("rejoin", false), msg.value("leave_shared", false));
    } else if (t == "REJECT") {
        std::string reason = msg.value("reason", std::string("rejected"));
        std::string detail = msg.value("detail", std::string());
        std::map<std::string, std::string> args;
        if (msg.contains("args") && msg["args"].is_object()) {
            for (auto& [k, v] : msg["args"].items()) {
                args[k] = v.is_string() ? v.get<std::string>() : v.dump();
            }
        }
        if (reason == "protocol_mismatch" && mProto == 5 && args.empty() &&
            Crypto::NormalizePassword(req.password).empty() && Crypto::NormalizePassword(req.serverPassword).empty()) {
            // An older server (protocol 4): once more with protocol 4, the room needs no password there (5, 1).
            Log("net: the server does not speak protocol 5 (" + detail + "): retrying with protocol 4");
            mProto = 4;
            mFallbackPending = true;
            return false;
        }
        SetState(NetState::Rejected, reason, detail);
        {
            std::lock_guard<std::mutex> lock(mMutex);
            mStatus.args = args;
        }
        std::string argText;
        for (auto& [k, v] : args) {
            argText += " " + k + "=" + v;
        }
        Log("net: REJECT reason=" + reason + " detail=" + detail + argText);
        rejected = true;
    } else if (t == "PLAYER_LIST") {
        {
            std::lock_guard<std::mutex> lock(mMutex);
            ReadPlayers(msg, mStatus.players);
            if (msg.contains("room_password")) {
                mStatus.roomPassword = msg.value("room_password", false);
            }
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
// What the status line and the log say of an address that could not be reached.
static std::string DialDetail(const Address& a, const std::string& reason) {
    if (reason == "tls_error") {
        return "El certificado de " + a.host + " no es valido (wss): no se conecta";
    }
    if (reason == "not_zmp_server") {
        return a.Text() + " responde, pero no es un servidor ZMP";
    }
    if (reason == "unsupported") {
        return "Las direcciones ws:// y wss:// necesitan la version de Windows o el navegador";
    }
    return "No se pudo conectar a " + a.Text();
}

void Client::Run(ConnectRequest req) {
    GetHandshake();
    const std::string& room = req.room;
    std::vector<Address> candidates;
    for (auto& text : req.addresses) {
        Address a = ParseAddress(text, req.defaultPort);
        if (a.ok) {
            candidates.push_back(a);
        } else {
            Log("net: bad address '" + text + "' (" + a.error + ")");
        }
    }
    if (candidates.empty()) {
        SetState(NetState::Rejected, "bad_address", "Direccion no valida: " + req.addresses[0]);
        return;
    }
    // (several addresses, from an invite code: 3 s each, in order; the first that connects is used)
    int timeout = candidates.size() > 1 ? 3000 : 10000;
    Address addr = candidates[0];

    while (!mStop) {
        SetState(NetState::Connecting);
        DialResult dial;
        for (size_t i = 0; i < candidates.size() && !mStop; i++) {
            addr = candidates[i];
            // A bare domain gets 3 s of TCP, then wss://domain/ (5.1); what worked is kept in `via`.
            dial = Dial(addr, addr.bareDomain ? 3000 : timeout);
            if (dial.conn == nullptr && addr.bareDomain && !mStop) {
                Address w = WssFallback(addr);
                Log("net: tcp to " + addr.Text() + " failed (" + dial.detail + "), trying " + w.Url());
                DialResult viaWss = Dial(w, 10000);
                if (viaWss.conn != nullptr) {
                    addr = w;
                    dial = std::move(viaWss);
                }
            }
            if (dial.conn != nullptr) {
                break;
            }
            if (candidates.size() > 1) {
                Log("net: " + addr.Text() + " did not answer (" + dial.detail + ")");
            }
        }
        if (dial.conn == nullptr) {
            SetState(NetState::Error, dial.reason, DialDetail(addr, dial.reason));
            {
                std::lock_guard<std::mutex> lock(mMutex);
                mStatus.args = { { "address", addr.Text() } };
            }
            Log("net: connect failed to " + addr.Text() + ": " + dial.detail);
            // Retry every 2 s until Disconnect() is called (a certificate refusal too: it says so each time).
            for (int i = 0; i < 20 && !mStop; i++) {
                std::this_thread::sleep_for(std::chrono::milliseconds(100));
            }
            continue;
        }
        Conn* conn = dial.conn.get();
        {
            std::lock_guard<std::mutex> lock(mMutex);
            mStatus.via = addr.Text();
            mStatus.transport = Address::KindName(addr.kind);
        }

        Log(std::string("net: ") + Address::KindName(addr.kind) + " connected to " + addr.Text() + ", sending HELLO");
        nlohmann::json hello = MakeHello(req);
        bool ok;
        {
            std::lock_guard<std::mutex> lock(mSendMutex);
            mSock = conn;
            ok = conn->SendFrame(EncodeFrame(hello));
        }
        {
            std::lock_guard<std::mutex> lock(sOut->mutex);
            sOut->frames.clear();
            sOut->open = ok;
        }
        bool welcomed = false;
        uint64_t lastPing = 0;
        uint64_t pingNonce = 0;
        bool rejected = false;

        while (ok && !mStop) {
            std::vector<nlohmann::json> msgs;
            bool alive;
            try {
                alive = conn->Poll(100, msgs);
                for (auto& msg : msgs) {
                    if (!HandleMessage(msg, req, welcomed, rejected)) {
                        ok = false;
                    }
                    if (rejected) {
                        break;
                    }
                }
            } catch (const std::exception& e) {
                Log(std::string("net: protocol error: ") + e.what());
                alive = false;
            }
            if (!alive) {
                ok = false;
            }
            if (rejected) {
                break;
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
        }
        conn->Close();
        dial.conn.reset();

        if (rejected || mStop) {
            break;
        }
        if (mFallbackPending) {
            mFallbackPending = false; // (straight away, with protocol 4)
            continue;
        }
        SetState(NetState::Error, "connection_lost", "Conexion perdida con " + addr.Text());
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
// The URL of the server, by the shared address grammar (Address.h): TCP addresses become ws://host:port/ (the
// server speaks both on one port, D-239), host:443 becomes wss://host/, ws:// and wss:// URLs are used as written.
static std::string WebUrl(const std::string& host, uint16_t port) {
    Address a = ParseAddress(host, port);
    return a.ok ? a.Url() : std::string();
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
    if (url.empty()) {
        SetState(NetState::Rejected, "bad_address", "Direccion no valida: " + sWeb.host);
        return;
    }
    {
        Address a = ParseAddress(sWeb.host, sWeb.port);
        std::lock_guard<std::mutex> lock(mMutex);
        mStatus.via = a.kind == Address::Kind::Tcp ? url : a.Text();
        mStatus.transport = a.kind == Address::Kind::Wss ? "wss" : "ws";
    }
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
    nlohmann::json hello = MakeHello(sWeb.req);
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
    bool ok = HandleMessage(msg, sWeb.req, sWeb.welcomed, sWeb.rejected);
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
    }
    if (mStop) {
        SetState(NetState::Disconnected);
        return;
    }
    if (sWeb.rejected) {
        return; // (state Rejected already set by HandleMessage)
    }
    if (mFallbackPending) {
        mFallbackPending = false; // (an older server: again at once with protocol 4)
        sWeb.retryAt = emscripten_get_now();
        return;
    }
    std::string where = sWeb.host + ":" + std::to_string(sWeb.port);
    if (!wasOpen && sWeb.addrIndex + 1 < sWeb.req.addresses.size()) {
        // (an invite code's next address, at once)
        Log("net: websocket connect failed to " + where + ", trying the next address of the code");
        sWeb.addrIndex++;
        sWeb.host = sWeb.req.addresses[sWeb.addrIndex];
        sWeb.retryAt = emscripten_get_now();
        return;
    }
    if (!wasOpen && sWeb.req.addresses.size() > 1) {
        sWeb.addrIndex = 0;
        sWeb.host = sWeb.req.addresses[0];
    }
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
