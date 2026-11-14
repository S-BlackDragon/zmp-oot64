#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#elif defined(_WIN32)
#ifndef NOMINMAX
#define NOMINMAX
#endif
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#pragma comment(lib, "ws2_32.lib")
#else
#include <fcntl.h>
#include <netdb.h>
#include <poll.h>
#include <sys/socket.h>
#include <unistd.h>
#endif

#include "LogShip.h"

#include <algorithm>
#include <atomic>
#include <chrono>
#include <condition_variable>
#include <cstdio>
#include <ctime>
#include <deque>
#include <filesystem>
#include <fstream>
#include <mutex>
#include <thread>
#include <vector>

#include <libultraship/bridge/consolevariablebridge.h>

#include "soh/Zmp/ZmpCVars.h"
#include "soh/Zmp/ZmpLog.h"

#ifdef ZMP_HAVE_ZLIB
#include <zlib.h>
#endif

namespace Zmp::LogShip {
namespace {

namespace fs = std::filesystem;

constexpr const char* kZmpLog = "logs/zmp.log";
constexpr const char* kGameLog = "logs/Ship of Harkinian.log";
constexpr const char* kGameLogOld = "logs/Ship of Harkinian.1.log"; // (spdlog's rotation renames the full log)
constexpr const char* kMarker = "logs/zmp-logship.json";
constexpr int64_t kChunk = 2 << 20;       // bytes of log per request (the server takes up to 8 MiB)
constexpr int64_t kMaxBacklog = 16 << 20; // a connection starts at most this far behind the end of a log
constexpr int kDefaultEverySeconds = 180;

// One log of one run: [start, end) of a file (end -1: the file grows, it is this run's), and what of it went out.
struct Track {
    std::string kind; // "zmp", "game" or "dump"
    std::string path;
    std::string run;
    std::string reason; // of the first upload of this piece ("previous", "crash", "desync"); "" the current reason
    int64_t start = 0;
    int64_t end = -1;
    int64_t shipped = 0; // bytes after start that are on some server
    // the current connection: the server has [connBase, connBase + connSent) of it in this connection's file
    uint64_t connGen = 0;
    int64_t connBase = 0;
    int64_t connSent = 0;
    bool once = false; // a dump: dropped once it is all out
    int findStart = 0; // the game's log of this run: tries left to find its first line (the game's logger is async)
};

struct State {
    std::timed_mutex mutex;
    std::string run;
    std::vector<Track> tracks;
    bool started = false;
    // connection
    bool collect = false;
    std::string token;
    std::string host;
    uint16_t port = 0;
    uint64_t gen = 0;
    bool offLogged = false;
    // what to do
    bool due = false;
    std::string reason = "periodic";
    double nextPeriodic = 0; // ms (steady)
    double retryAt = 0;
    double backoff = 60000;
    bool failing = false;
    // numbers for the harness
    int ok = 0;
    int failed = 0;
    int64_t bytes = 0;
    int lastStatus = 0;
    std::string lastReason;
    std::string lastError;
    // native: the uploader thread
    std::thread thread;
    std::condition_variable_any cv;
    std::atomic<bool> stop{ false };
    std::atomic<bool> busy{ false };
    bool inFlight = false;             // web
    std::vector<std::string> webRound; // web: the tracks this round has sent to their end
    // the settings, read on the game thread (Poll) and used by the uploader
    std::atomic<bool> setting{ true };
    std::atomic<int> everySeconds{ kDefaultEverySeconds };
};

State& S() {
    static State s;
    return s;
}

double NowMs() {
#ifdef __EMSCRIPTEN__
    return emscripten_get_now();
#else
    return (double)std::chrono::duration_cast<std::chrono::milliseconds>(
               std::chrono::steady_clock::now().time_since_epoch())
        .count();
#endif
}

int64_t FileSize(const std::string& path) {
    std::error_code ec;
    auto n = fs::file_size(path, ec);
    return ec ? -1 : (int64_t)n;
}

bool Enabled(const State& s) {
    return s.collect && !s.token.empty() && s.setting.load();
}

int EverySeconds() {
    return S().everySeconds.load();
}

// The settings, on the game thread (the console variables are not read from other threads).
void ReadSettings() {
    State& s = S();
    s.setting = CVarGetInteger(ZMP_CVAR_LOGS_UPLOAD, 1) != 0;
    int v = CVarGetInteger(ZMP_CVAR_LOGS_UPLOAD_EVERY, kDefaultEverySeconds);
    s.everySeconds = v < 2 ? 2 : v;
}

// Where this run starts in the game's log: the last "Starting Ship of Harkinian" line (the game's first line of a
// run, OTRGlobals.cpp), looked for in the last 8 MiB; without it, the size now (what this run logged before ZMP
// started is then not sent, and the log says so).
int64_t GameRunStart(bool& found) {
    found = false;
    int64_t size = FileSize(kGameLog);
    if (size <= 0) {
        return 0;
    }
    int64_t from = std::max<int64_t>(0, size - (8 << 20));
    std::ifstream f(kGameLog, std::ios::binary);
    if (!f) {
        return size;
    }
    f.seekg(from);
    std::string text((std::istreambuf_iterator<char>(f)), std::istreambuf_iterator<char>());
    size_t at = text.rfind("] Starting Ship of Harkinian");
    if (at == std::string::npos) {
        return size;
    }
    size_t line = text.rfind('\n', at);
    found = true;
    return from + (int64_t)(line == std::string::npos ? 0 : line + 1);
}

std::string RunId() {
    std::time_t t = std::time(nullptr);
    std::tm tmNow{};
#ifdef _WIN32
    localtime_s(&tmNow, &t);
#else
    localtime_r(&t, &tmNow);
#endif
    char buf[32];
    std::strftime(buf, sizeof(buf), "%Y%m%d-%H%M%S", &tmNow);
#ifdef __EMSCRIPTEN__
    return std::string(buf) + "-web";
#elif defined(_WIN32)
    return std::string(buf) + "-" + std::to_string(GetCurrentProcessId() % 100000);
#else
    return std::string(buf) + "-" + std::to_string(getpid() % 100000);
#endif
}

nlohmann::json TrackJson(const Track& t) {
    return { { "kind", t.kind },   { "path", t.path }, { "run", t.run },         { "reason", t.reason },
             { "start", t.start }, { "end", t.end },   { "shipped", t.shipped }, { "once", t.once } };
}

// The marker of this run (logs/zmp-logship.json): what the next start needs to send what this one did not.
void SaveMarker(State& s, const char* runState) {
#ifndef __EMSCRIPTEN__ // (the web's files do not outlive the page)
    nlohmann::json j = { { "run", s.run }, { "state", runState }, { "tracks", nlohmann::json::array() } };
    for (auto& t : s.tracks) {
        j["tracks"].push_back(TrackJson(t));
    }
    std::string tmp = std::string(kMarker) + ".tmp";
    {
        std::ofstream f(tmp, std::ios::binary | std::ios::trunc);
        f << j.dump();
    }
    std::error_code ec;
    fs::rename(tmp, kMarker, ec);
#endif
}

std::string& RunState() {
    static std::string st = "running";
    return st;
}

// ---- One upload ---------------------------------------------------------------------------------------------------

struct Request {
    uint64_t gen = 0;
    size_t track = 0;
    std::string kind;
    std::string run;
    std::string reason;
    int64_t offset = 0; // in this connection's file on the server
    int64_t from = 0;   // absolute offset in the local file
    std::string host;
    uint16_t port = 0;
    std::string token;
    std::vector<uint8_t> body;
    int64_t rawLen = 0; // bytes of log in body (before gzip)
    bool gz = false;
};

bool ReadRange(const std::string& path, int64_t from, int64_t len, std::vector<uint8_t>& out) {
    out.clear();
    if (len <= 0) {
        return true;
    }
    FILE* f = fopen(path.c_str(), "rb");
    if (f == nullptr) {
        return false;
    }
#ifdef _WIN32
    _fseeki64(f, from, SEEK_SET);
#else
    fseeko(f, (off_t)from, SEEK_SET);
#endif
    out.resize((size_t)len);
    size_t n = fread(out.data(), 1, (size_t)len, f);
    fclose(f);
    out.resize(n);
    return true;
}

#ifdef ZMP_HAVE_ZLIB
bool Gzip(const std::vector<uint8_t>& in, std::vector<uint8_t>& out) {
    z_stream z{};
    if (deflateInit2(&z, Z_DEFAULT_COMPRESSION, Z_DEFLATED, 15 + 16, 8, Z_DEFAULT_STRATEGY) != Z_OK) {
        return false;
    }
    out.resize(deflateBound(&z, (uLong)in.size()) + 32);
    z.next_in = (Bytef*)in.data();
    z.avail_in = (uInt)in.size();
    z.next_out = out.data();
    z.avail_out = (uInt)out.size();
    int r = deflate(&z, Z_FINISH);
    out.resize(z.total_out);
    deflateEnd(&z);
    return r == Z_STREAM_END;
}
#endif

// The next piece to send, if any (the state's mutex held). A track whose server copy is behind by more than
// kMaxBacklog starts this connection near the end (the tail is what tells a failure; the skip is logged).
// `done`: the tracks this round already sent up to their end (one pass per round: the lines an upload itself makes
// wait for the next round instead of a chain of tiny uploads). nullptr: no rounds.
bool NextRequest(State& s, Request& r, std::vector<std::string>* done = nullptr) {
    if (!Enabled(s)) {
        return false;
    }
    for (size_t i = 0; i < s.tracks.size(); i++) {
        Track& t = s.tracks[i];
        if (done != nullptr && std::find(done->begin(), done->end(), t.path + "|" + t.run) != done->end()) {
            continue;
        }
        int64_t size = FileSize(t.path);
        if (size < 0) {
            continue;
        }
        if (t.findStart > 0 && t.connGen == 0) {
            // (at Zmp_Init the game's asynchronous logger may not have written this run's first line yet)
            bool found = false;
            int64_t at = GameRunStart(found);
            t.findStart--;
            if (found && at <= t.start && t.start - at < (4 << 20)) {
                Log("logship: this run's game log starts " + std::to_string(t.start - at) + " bytes earlier");
                t.start = at;
                t.findStart = 0;
            }
        }
        int64_t end = t.end >= 0 ? std::min(t.end, size) : size;
        if (end < t.start) { // (the file was cut or rotated under us: start over from its beginning)
            t.start = 0;
            t.shipped = 0;
            t.connGen = 0;
        }
        int64_t have = end - t.start;
        if (t.connGen != s.gen) {
            t.connGen = s.gen;
            t.connBase = std::min(t.shipped, have);
            t.connSent = 0;
            if (have - t.connBase > kMaxBacklog) {
                Log("logship: " + t.kind + " log of run " + t.run + " is " + std::to_string((have - t.connBase) >> 20) +
                    " MiB behind: only its last " + std::to_string(kMaxBacklog >> 20) + " MiB go to this server");
                t.connBase = have - kMaxBacklog;
            }
        }
        int64_t at = t.connBase + t.connSent;
        if (at >= have) {
            continue;
        }
        r.gen = s.gen;
        r.track = i;
        r.kind = t.kind;
        r.run = t.run;
        r.reason = !t.reason.empty() ? t.reason : s.reason;
        r.offset = t.connSent;
        r.from = t.start + at;
        r.host = s.host;
        r.port = s.port;
        r.token = s.token;
        std::vector<uint8_t> raw;
        if (!ReadRange(t.path, r.from, std::min(kChunk, have - at), raw) || raw.empty()) {
            continue;
        }
        if (done != nullptr && (int64_t)raw.size() < kChunk) {
            done->push_back(t.path + "|" + t.run); // (this piece reaches its end)
        }
        r.gz = false;
        r.rawLen = (int64_t)raw.size();
#ifdef ZMP_HAVE_ZLIB
        if (Gzip(raw, r.body)) {
            r.gz = true;
        } else {
            r.body = std::move(raw);
        }
#else
        r.body = std::move(raw);
#endif
        return true;
    }
    return false;
}

std::string QueryOf(const Request& r) {
    return std::string("/zmp/logs?kind=") + r.kind + "&run=" + r.run + "&reason=" + r.reason +
           "&offset=" + std::to_string(r.offset) + (r.gz ? "&gz=1" : "");
}

// The answer to one request (the state's mutex held). status 0: no answer (network).
void Complete(State& s, const Request& r, int status, int64_t serverSize, const std::string& error) {
    s.lastStatus = status;
    if (r.gen != s.gen || r.track >= s.tracks.size()) {
        return; // (an answer for an older connection)
    }
    Track& t = s.tracks[r.track];
    if (status == 200 || (status == 409 && serverSize >= 0)) {
        if (status == 200) {
            s.ok++;
            s.bytes += (int64_t)r.body.size();
            s.lastReason = r.reason;
        }
        if (serverSize < 0 && status == 200) { // (an answer without the size: what was sent is there)
            serverSize = r.offset + r.rawLen;
        }
        t.connSent = std::max<int64_t>(0, serverSize);
        t.shipped = std::max(t.shipped, t.connBase + t.connSent);
        if (!t.reason.empty() && t.end >= 0 && t.shipped >= t.end - t.start) {
            Log("logship: " + t.kind + " log of run " + t.run + " (" + t.reason + ") is on the server");
        }
        if (s.failing) {
            Log("logship: uploads work again");
        }
        s.failing = false;
        s.backoff = 60000;
        return;
    }
    s.failed++;
    s.lastError = error;
    if (!s.failing) {
        Log("logship: upload of the " + r.kind + " log failed (" +
            (status == 0 ? error : "HTTP " + std::to_string(status)) + "); retried later");
    }
    s.failing = true;
    double wait = status == 429 ? 30 * 60000.0 : s.backoff;
    s.retryAt = NowMs() + wait;
    s.backoff = std::min(s.backoff * 2, 600000.0);
    if (status == 403 || status == 404) { // (the token is gone, or the server stopped collecting: next WELCOME)
        Log("logship: the server refused the upload (HTTP " + std::to_string(status) + "): nothing more is sent " +
            "until the next connection");
        s.collect = false;
    }
}

// After every completed piece: drop the dumps that are out and keep the marker up to date.
void AfterPieces(State& s) {
    s.tracks.erase(std::remove_if(s.tracks.begin(), s.tracks.end(),
                                  [](const Track& t) { return t.once && t.end >= 0 && t.shipped >= t.end - t.start; }),
                   s.tracks.end());
    SaveMarker(s, RunState().c_str());
}

int64_t JsonSize(const std::string& body) {
    try {
        auto j = nlohmann::json::parse(body);
        return j.value("size", (int64_t)-1);
    } catch (...) { return -1; }
}

#ifndef __EMSCRIPTEN__
// ---- Native: a minimal HTTP/1.1 POST over a socket with timeouts -------------------------------------------------

#ifdef _WIN32
using Sock = SOCKET;
constexpr Sock kBad = INVALID_SOCKET;
void CloseSock(Sock k) {
    closesocket(k);
}
#else
using Sock = int;
constexpr Sock kBad = -1;
void CloseSock(Sock k) {
    close(k);
}
#endif

bool WaitSock(Sock k, bool write, int ms) {
#ifdef _WIN32
    WSAPOLLFD p{ k, (SHORT)(write ? POLLWRNORM : POLLRDNORM), 0 };
    return WSAPoll(&p, 1, ms) > 0;
#else
    pollfd p{ k, (short)(write ? POLLOUT : POLLIN), 0 };
    return poll(&p, 1, ms) > 0;
#endif
}

Sock Dial(const std::string& host, uint16_t port, int timeoutMs, std::string& error) {
#ifdef _WIN32
    static bool wsa = [] {
        WSADATA d;
        return WSAStartup(MAKEWORD(2, 2), &d) == 0;
    }();
    (void)wsa;
#endif
    addrinfo hints{};
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    addrinfo* res = nullptr;
    if (getaddrinfo(host.c_str(), std::to_string(port).c_str(), &hints, &res) != 0 || res == nullptr) {
        error = "cannot resolve " + host;
        return kBad;
    }
    Sock k = kBad;
    for (addrinfo* a = res; a != nullptr && k == kBad; a = a->ai_next) {
        Sock c = socket(a->ai_family, a->ai_socktype, a->ai_protocol);
        if (c == kBad) {
            continue;
        }
#ifdef _WIN32
        u_long nb = 1;
        ioctlsocket(c, FIONBIO, &nb);
#else
        fcntl(c, F_SETFL, fcntl(c, F_GETFL, 0) | O_NONBLOCK);
#endif
        connect(c, a->ai_addr, (int)a->ai_addrlen);
        bool up = WaitSock(c, true, timeoutMs);
        int soErr = 0;
#ifdef _WIN32
        int len = sizeof(soErr);
        getsockopt(c, SOL_SOCKET, SO_ERROR, (char*)&soErr, &len);
        u_long block = 0;
        ioctlsocket(c, FIONBIO, &block);
#else
        socklen_t len = sizeof(soErr);
        getsockopt(c, SOL_SOCKET, SO_ERROR, &soErr, &len);
        fcntl(c, F_SETFL, fcntl(c, F_GETFL, 0) & ~O_NONBLOCK);
#endif
        if (up && soErr == 0) {
            k = c;
        } else {
            CloseSock(c);
        }
    }
    freeaddrinfo(res);
    if (k == kBad) {
        error = "cannot connect to " + host + ":" + std::to_string(port);
    }
    return k;
}

bool SendAllSock(Sock k, const char* data, size_t len, int timeoutMs) {
    while (len > 0) {
        if (!WaitSock(k, true, timeoutMs)) {
            return false;
        }
        int n = send(k, data, (int)std::min<size_t>(len, 1 << 20), 0);
        if (n <= 0) {
            return false;
        }
        data += n;
        len -= (size_t)n;
    }
    return true;
}

// Returns the HTTP status (0 on a network failure, with error) and the answer's body.
int HttpPost(const Request& r, int connectMs, int ioMs, std::string& answer, std::string& error) {
    Sock k = Dial(r.host, r.port, connectMs, error);
    if (k == kBad) {
        return 0;
    }
    std::string head =
        "POST " + QueryOf(r) + " HTTP/1.1\r\nHost: " + r.host + ":" + std::to_string(r.port) +
        "\r\nAuthorization: Bearer " + r.token +
        "\r\nContent-Type: application/octet-stream\r\nContent-Length: " + std::to_string(r.body.size()) +
        "\r\nConnection: close\r\n\r\n";
    // (the head is sent whole in one write: the server tells HTTP from the game's protocol by the first bytes)
    if (!SendAllSock(k, head.data(), head.size(), ioMs) ||
        !SendAllSock(k, (const char*)r.body.data(), r.body.size(), ioMs)) {
        CloseSock(k);
        error = "send failed";
        return 0;
    }
    std::string in;
    char buf[4096];
    while (in.size() < (1 << 16) && WaitSock(k, false, ioMs)) {
        int n = recv(k, buf, sizeof(buf), 0);
        if (n <= 0) {
            break;
        }
        in.append(buf, (size_t)n);
    }
    CloseSock(k);
    int status = 0;
    if (in.rfind("HTTP/1.", 0) != 0 || sscanf(in.c_str(), "HTTP/1.%*d %d", &status) != 1) {
        error = in.empty() ? "no answer" : "not an HTTP answer";
        return 0;
    }
    size_t body = in.find("\r\n\r\n");
    answer = body == std::string::npos ? std::string() : in.substr(body + 4);
    return status;
}

// Sends everything there is to send now (the uploader thread, or the crash handler with short timeouts).
void Drain(State& s, int connectMs, int ioMs, double deadline) {
    std::vector<std::string> done;
    for (;;) {
        Request r;
        {
            std::unique_lock<std::timed_mutex> lock(s.mutex);
            if (!NextRequest(s, r, &done)) {
                s.due = false;
                s.reason = "periodic";
                return;
            }
        }
        std::string answer;
        std::string error;
        int status = HttpPost(r, connectMs, ioMs, answer, error);
        std::unique_lock<std::timed_mutex> lock(s.mutex);
        Complete(s, r, status, JsonSize(answer), error);
        AfterPieces(s);
        if ((status != 200 && status != 409) || (deadline > 0 && NowMs() > deadline)) {
            return;
        }
    }
}

void Worker() {
    State& s = S();
    while (!s.stop) {
        {
            std::unique_lock<std::timed_mutex> lock(s.mutex);
            s.cv.wait_for(lock, std::chrono::milliseconds(500));
            double now = NowMs();
            if (Enabled(s) && now >= s.nextPeriodic) {
                s.nextPeriodic = now + EverySeconds() * 1000.0;
                s.due = true;
            }
            if (!s.due || !Enabled(s) || now < s.retryAt) {
                continue;
            }
        }
        s.busy = true;
        Drain(s, 3000, 10000, 0);
        s.busy = false;
    }
}
#endif // !__EMSCRIPTEN__

#ifdef __EMSCRIPTEN__
// ---- Web: the browser's fetch (CORS: the server answers it), gzip by CompressionStream where there is one --------
Request sWebReq;

// clang-format off
EM_JS(void, zmp_logship_post, (const char* url, const char* token, const uint8_t* data, int len), {
    var u = UTF8ToString(url);
    var tok = UTF8ToString(token);
    var bytes = HEAPU8.slice(data, data + len);
    var done = function(status, text) {
        var size = -1;
        try { size = JSON.parse(text).size; } catch (e) {}
        _Zmp_LogShipDone(status, typeof size === "number" ? size : -1);
    };
    var send = function(body, gz) {
        fetch(u + (gz ? "&gz=1" : ""), { method: "POST", headers: { "Authorization": "Bearer " + tok }, body: body })
            .then(function(r) { return r.text().then(function(t) { done(r.status, t); }); })
            .catch(function(e) { done(0, ""); });
    };
    if (typeof CompressionStream !== "undefined") {
        new Response(new Blob([bytes]).stream().pipeThrough(new CompressionStream("gzip"))).arrayBuffer()
            .then(function(b) { send(b, true); })
            .catch(function() { send(bytes, false); });
    } else {
        send(bytes, false);
    }
});
// clang-format on

// The http(s) address of the server's port, from what the web client connected to (ws://host:port/ or a wss:// URL).
std::string WebBase(const std::string& host, uint16_t port) {
    std::string u;
    if (host.rfind("ws://", 0) == 0) {
        u = "http://" + host.substr(5);
    } else if (host.rfind("wss://", 0) == 0) {
        u = "https://" + host.substr(6);
    } else if (port == 443) {
        u = "https://" + host;
    } else {
        u = "http://" + host + ":" + std::to_string(port);
    }
    size_t scheme = u.find("://") + 3;
    size_t path = u.find('/', scheme);
    return path == std::string::npos ? u : u.substr(0, path);
}
#endif

} // namespace

#ifdef __EMSCRIPTEN__
extern "C" EMSCRIPTEN_KEEPALIVE void Zmp_LogShipDone(int status, double size) {
    State& s = S();
    std::unique_lock<std::timed_mutex> lock(s.mutex);
    s.inFlight = false;
    Complete(s, sWebReq, status, (int64_t)size, status == 0 ? std::string("fetch failed") : std::string());
    AfterPieces(s);
    if (status != 200 && status != 409) {
        s.due = false;
    }
}
#endif

void Start() {
    State& s = S();
    std::unique_lock<std::timed_mutex> lock(s.mutex);
    if (s.started) {
        return;
    }
    s.started = true;
    ReadSettings();
    s.run = RunId();
    std::error_code ec;
    fs::create_directories("logs", ec);
    // what the previous run of the game in this folder left unsent (its marker), sent on the first WELCOME
    std::vector<Track> previous;
    std::string prevNote;
#ifndef __EMSCRIPTEN__
    int64_t zmpStart = std::max<int64_t>(0, FileSize(kZmpLog));
    bool gameFound = false;
    int64_t gameStart = GameRunStart(gameFound);
    try {
        std::ifstream f(kMarker, std::ios::binary);
        if (f) {
            auto j = nlohmann::json::parse(f);
            std::string prevRun = j.value("run", std::string());
            std::string prevState = j.value("state", std::string());
            std::string why = prevState == "clean" ? "previous" : "crash";
            for (auto& tj : j.value("tracks", nlohmann::json::array())) {
                Track t;
                t.kind = tj.value("kind", std::string());
                t.path = tj.value("path", std::string());
                t.run = tj.value("run", prevRun);
                t.start = tj.value("start", (int64_t)0);
                t.end = tj.value("end", (int64_t)-1);
                t.shipped = tj.value("shipped", (int64_t)0);
                t.once = tj.value("once", false);
                t.reason = tj.value("reason", std::string());
                if (t.run.rfind(prevRun, 0) != 0) {
                    continue; // (an older run's leftover: only the last run is kept)
                }
                t.once = true;   // (dropped from the list once it is all out)
                if (t.end < 0) { // the previous run's own log ends where this run starts
                    if (t.path == kZmpLog) {
                        t.end = zmpStart;
                    } else if (t.path == kGameLog) {
                        if (FileSize(kGameLog) < t.start) { // (rotated since: the previous log is the .1 file)
                            t.path = kGameLogOld;
                            t.end = std::max<int64_t>(0, FileSize(kGameLogOld));
                        } else {
                            t.end = gameStart;
                        }
                    }
                }
                if (t.reason.empty()) {
                    t.reason = why;
                }
                if (t.end - t.start > t.shipped) {
                    previous.push_back(t);
                }
            }
            if (!previous.empty()) {
                prevNote = "the previous run " + prevRun + " ended " +
                           (prevState == "clean" ? "cleanly" : "without a clean exit (" + prevState + ")") +
                           " with logs unsent: they go to the next server that collects logs";
            }
        }
    } catch (...) { prevNote = "the marker of the previous run could not be read"; }
    s.tracks.push_back(Track{ "zmp", kZmpLog, s.run, "", zmpStart });
    s.tracks.push_back(Track{ "game", kGameLog, s.run, "", gameStart });
    s.tracks.back().findStart = gameFound ? 0 : 3;
    for (auto& t : previous) {
        s.tracks.push_back(t);
    }
    SaveMarker(s, "running");
    lock.unlock();
    Log("logship: run " + s.run + (gameFound ? "" : " (this run's first game log line is not written yet)"));
    if (!prevNote.empty()) {
        Log("logship: " + prevNote);
    }
    s.thread = std::thread(Worker);
#else
    s.tracks.push_back(Track{ "zmp", kZmpLog, s.run, "", std::max<int64_t>(0, FileSize(kZmpLog)) });
    lock.unlock();
    Log("logship: run " + s.run + " (web: the ZMP log of this page)");
#endif
}

void OnWelcome(bool collect, const std::string& token, const std::string& host, uint16_t port) {
    State& s = S();
    bool setting = s.setting.load();
    {
        std::unique_lock<std::timed_mutex> lock(s.mutex);
        s.collect = collect;
        s.token = token;
        s.host = host;
        s.port = port;
        s.gen++;
        s.failing = false;
        s.retryAt = 0;
        s.backoff = 60000;
        s.due = Enabled(s); // (the first upload of a connection: the previous run's logs, and this run's so far)
        s.reason = "periodic";
        s.nextPeriodic = NowMs() + EverySeconds() * 1000.0;
    }
    if (!collect) {
        Log("logship: this server does not collect logs: nothing is sent to it");
    } else if (!setting) {
        Log("logship: the server collects logs, but the setting gZmp.Logs.Upload is off: nothing is sent");
    } else {
        Log("logship: the server collects logs: this run's logs go to it every " + std::to_string(EverySeconds()) +
            " s, at the end, on a desync and on a crash");
    }
    s.cv.notify_all();
}

void OnSessionEnd() {
    State& s = S();
    {
        std::unique_lock<std::timed_mutex> lock(s.mutex);
        if (!Enabled(s)) {
            return;
        }
        s.due = true;
        s.reason = "end";
        s.retryAt = 0;
    }
    Log("logship: session over: its logs go to the server");
    s.cv.notify_all();
}

void OnDesync(uint32_t badTick) {
    State& s = S();
    {
        std::unique_lock<std::timed_mutex> lock(s.mutex);
        if (!Enabled(s)) {
            return;
        }
        std::string dump = "logs/desync-" + std::to_string(badTick) + "-exact.txt";
        if (FileSize(dump) <= 0) {
            dump = "logs/desync-" + std::to_string(badTick) + ".txt";
        }
        int64_t size = FileSize(dump);
        if (size > 0) {
            Track t{ "dump", dump, s.run + "-t" + std::to_string(badTick), "desync", 0, size };
            t.once = true;
            s.tracks.push_back(t);
        }
        s.due = true;
        s.reason = "desync";
        s.retryAt = 0;
    }
    Log("logship: desync at tick " + std::to_string(badTick) + ": the logs go to the server");
    s.cv.notify_all();
}

void OnCrash() {
    State& s = S();
    RunState() = "crashed";
    // (the uploader may hold the lock in the middle of a request: wait for it a little, never for ever)
    std::unique_lock<std::timed_mutex> lock(s.mutex, std::chrono::milliseconds(1500));
    if (!lock.owns_lock()) {
        return;
    }
    for (auto& t : s.tracks) {
        if (t.end < 0) {
            t.reason = "crash";
        }
    }
    s.reason = "crash";
    SaveMarker(s, "crashed");
    bool on = Enabled(s);
    lock.unlock();
#ifndef __EMSCRIPTEN__
    if (on) {
        Log("logship: crash: sending this run's logs");
        Drain(s, 1500, 3000, NowMs() + 5000);
    }
#endif
}

void Stop() {
    State& s = S();
    if (!s.started) {
        return;
    }
#ifndef __EMSCRIPTEN__
    // the last upload (OnSessionEnd asked for it): at most 4 s more, then the next start sends the rest
    double until = NowMs() + 4000;
    for (;;) {
        bool pending;
        {
            std::unique_lock<std::timed_mutex> lock(s.mutex);
            Request r;
            pending = s.busy || (s.due && Enabled(s) && NowMs() >= s.retryAt && NextRequest(s, r));
        }
        if (!pending || NowMs() >= until) {
            break;
        }
        s.cv.notify_all();
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
    }
    s.stop = true;
    s.cv.notify_all();
    if (s.thread.joinable()) {
        s.thread.join();
    }
#endif
    std::unique_lock<std::timed_mutex> lock(s.mutex);
    RunState() = "clean";
    SaveMarker(s, "clean");
}

void Poll() {
    ReadSettings();
#ifdef __EMSCRIPTEN__
    State& s = S();
    std::unique_lock<std::timed_mutex> lock(s.mutex);
    if (!s.started || s.inFlight || !Enabled(s)) {
        return;
    }
    double now = NowMs();
    if (now >= s.nextPeriodic) {
        s.nextPeriodic = now + EverySeconds() * 1000.0;
        s.due = true;
    }
    if (!s.due || now < s.retryAt) {
        return;
    }
    Request r;
    if (!NextRequest(s, r, &s.webRound)) {
        s.due = false;
        s.reason = "periodic";
        s.webRound.clear();
        return;
    }
    sWebReq = std::move(r); // (not packed here: the page packs it and adds gz=1 when it can)
    s.inFlight = true;
    std::string url = WebBase(sWebReq.host, sWebReq.port) + QueryOf(sWebReq);
    zmp_logship_post(url.c_str(), sWebReq.token.c_str(), sWebReq.body.data(), (int)sWebReq.body.size());
#endif
}

nlohmann::json Status() {
    State& s = S();
    std::unique_lock<std::timed_mutex> lock(s.mutex);
    nlohmann::json tracks = nlohmann::json::array();
    for (auto& t : s.tracks) {
        tracks.push_back(TrackJson(t));
    }
    return { { "run", s.run },
             { "collect", s.collect },
             { "setting", s.setting.load() },
             { "enabled", Enabled(s) },
             { "uploads_ok", s.ok },
             { "uploads_failed", s.failed },
             { "bytes", s.bytes },
             { "last_status", s.lastStatus },
             { "last_reason", s.lastReason },
             { "last_error", s.lastError },
             { "due", s.due },
             { "tracks", tracks } };
}

} // namespace Zmp::LogShip
