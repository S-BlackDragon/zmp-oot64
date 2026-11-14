#pragma once

#include <atomic>
#include <map>
#include <cstdint>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

#include <nlohmann/json.hpp>

namespace Zmp {

enum class NetState { Disconnected, Connecting, Connected, Rejected, Error };

struct NetPlayer {
    uint32_t id = 0;
    std::string name;
    int slot = -1;
    int rtt = -1;
    std::string state;
    bool leader = false;
    int color = -1; // protocol 5: the tunic colour the player chose (0xRRGGBB), -1 = the slot's
};

struct NetStatus {
    NetState state = NetState::Disconnected;
    std::string host;  // what the player typed (any form of Address.h), or the bare host
    uint16_t port = 0; // the default port when the address has none
    // The address in use, as Address::Text() writes it, and its transport ("tcp", "ws", "wss"): what the status line
    // shows and the recent servers list keeps (a bare domain that fell back to wss keeps the wss form).
    std::string via;
    std::string transport;
    std::string room;
    std::string name;
    // Reject reason code ("build_mismatch", ...) or error code.
    std::string reason;
    std::string detail;
    uint32_t playerId = 0;
    std::vector<NetPlayer> players;
    int rttMs = -1;
    // Protocol 5 (docs/PROTOCOLO_5.md): the REJECT's args, what the WELCOME said of the room, the protocol in use.
    std::map<std::string, std::string> args;
    bool roomPassword = false;
    bool roomOwner = false;
    std::string serverName;
    int proto = 0;
    // The answer to the last Rename(): "" (none yet), "ok", or the reason ("name_taken", "bad_name"); renameSeq
    // counts the answers.
    std::string renameResult;
    uint32_t renameSeq = 0;
};

// What a connection needs (docs/PRODUCTO_PUBLICO.md I2). `addresses` are tried in order (an invite code carries one
// to four); each is any form of Address.h.
struct ConnectRequest {
    std::vector<std::string> addresses;
    uint16_t defaultPort = 47100;
    std::string room;
    std::string name;
    std::string password;       // the room's (empty: none); never logged, kept in memory only
    std::string serverPassword; // the server's, when it has one
    // (filled by Connect on the game thread)
    std::string playerKey;
    int color = -1;
};

struct Handshake {
    std::string buildHash;
    std::string ootHash;
    std::string sohHash;
    std::string cvarProfileHash;
    uint32_t randoSeed = 0;
    // The ROM version the assets were made from, by name ("PAL GameCube Debug", "NTSC N64 1.2"...): only for the
    // server's reject message (phase 5b). The check is on the hashes.
    std::string romName;
};

// Name of a ROM version by its CRC (the `version` entry of oot.o2r, soh/GameVersions.h).
std::string RomVersionName(uint32_t crc);

// Connection to zmp-server. Phase 0: handshake, room membership, ping. All network I/O
// runs on its own thread; nothing here writes game state.
class Client {
  public:
    static Client& Get();

    // `host` is any address of the grammar in Address.h (host, host:port, [IPv6]:port, ws://, wss://); `port` is the
    // port of an address that has none.
    void Connect(const std::string& host, uint16_t port, const std::string& room, const std::string& name);
    void Connect(ConnectRequest req);
    void Disconnect();
    // Protocol 5, while connected: another name (RENAME; the answer lands in GetStatus().renameResult) and another
    // tunic colour (SET_COLOR, 0xRRGGBB or -1). False when not connected with protocol 5.
    bool Rename(const std::string& name);
    bool SetColor(int rgb);
    // This player's key (32 hex characters, 16 random bytes made on first use and kept in the config, never derived
    // from the PC). Game thread only.
    static std::string PlayerKey();
    NetStatus GetStatus();
    // Handshake values of this client (file hashes are computed once, on first use).
    Handshake GetHandshake();

    static const char* StateName(NetState state);
    // Sends one message on the current connection (any thread). False if not connected.
    bool Send(const nlohmann::json& msg);

#ifdef __EMSCRIPTEN__
    // Web (D-239): the browser has no threads and no TCP; the connection is a WebSocket whose callbacks run on the
    // main thread. PollWeb runs once per frame (Zmp_OnFrameBegin) for the reconnection timer and the pings. The
    // three Web* methods are the socket's callbacks.
    void PollWeb();
    void WebOpen();
    void WebMessage(nlohmann::json& msg);
    void WebClosed(bool error);
#endif

  private:
    void Run(ConnectRequest req);
    nlohmann::json MakeHello(const ConnectRequest& req);
    void SetState(NetState state, const std::string& reason = "", const std::string& detail = "");
    // One message from the server, whichever transport brought it. False when an answer could not be sent.
    bool HandleMessage(nlohmann::json& msg, const ConnectRequest& req, bool& welcomed, bool& rejected);
#ifdef __EMSCRIPTEN__
    void WebStart();
#endif

    std::mutex mMutex;
    NetStatus mStatus;
    std::thread mThread;
    std::atomic<bool> mStop{ false };
    std::mutex mHandshakeMutex;
    bool mHandshakeReady = false;
    Handshake mHandshake;
    std::mutex mSendMutex;
    // Protocol 5 entry of the current connection (network thread; the web's main thread): the HELLO's auth_nonce,
    // the CHALLENGE's nonce, the protocol offered (5, or 4 once against an older server), and the room key cache.
    std::vector<uint8_t> mAuthNonce;
    std::vector<uint8_t> mServerNonce;
    bool mSentRoomProof = false;
    int mProto = 5;
    bool mFallbackPending = false;
    std::string mKeyFor;
    std::vector<uint8_t> mRoomKey;
    void* mSock = nullptr; // Conn* of the current connection (Transport.h; guarded by mSendMutex)
};

} // namespace Zmp
