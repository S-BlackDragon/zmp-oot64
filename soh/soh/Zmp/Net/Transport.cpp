#ifndef __EMSCRIPTEN__

#include "Transport.h"

#include <chrono>
#include <condition_variable>
#include <cstring>
#include <deque>
#include <mutex>
#include <thread>

#include "ZmpProtocol.h"
#include "soh/Zmp/ZmpLog.h"

#ifdef _WIN32
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <winhttp.h>
#pragma comment(lib, "ws2_32.lib")
#pragma comment(lib, "winhttp.lib")
typedef SOCKET SockT;
#define ZMP_BAD_SOCK INVALID_SOCKET
#define ZmpCloseSock closesocket
#else
#include <arpa/inet.h>
#include <fcntl.h>
#include <netdb.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <sys/select.h>
#include <sys/socket.h>
#include <unistd.h>
typedef int SockT;
#define ZMP_BAD_SOCK (-1)
#define ZmpCloseSock close
#endif

namespace Zmp {

namespace {

void SetBlocking(SockT s, bool blocking) {
#ifdef _WIN32
    u_long mode = blocking ? 0 : 1;
    ioctlsocket(s, FIONBIO, &mode);
#else
    int fl = fcntl(s, F_GETFL, 0);
    fcntl(s, F_SETFL, blocking ? (fl & ~O_NONBLOCK) : (fl | O_NONBLOCK));
#endif
}

// ---- TCP -----------------------------------------------------------------------------------------------------------
class TcpConn : public Conn {
  public:
    explicit TcpConn(SockT s) : mSock(s) {
    }
    ~TcpConn() override {
        Close();
    }
    bool SendFrame(const std::vector<uint8_t>& frame) override {
        size_t done = 0;
        while (done < frame.size()) {
            int n = send(mSock, (const char*)frame.data() + done, (int)(frame.size() - done), 0);
            if (n <= 0) {
                return false;
            }
            done += (size_t)n;
        }
        return true;
    }
    bool Poll(int ms, std::vector<nlohmann::json>& out) override {
        fd_set rd;
        FD_ZERO(&rd);
        FD_SET(mSock, &rd);
        timeval tv{ ms / 1000, (ms % 1000) * 1000 };
        int ready = select((int)mSock + 1, &rd, nullptr, nullptr, &tv);
        if (ready < 0) {
            return false;
        }
        if (ready == 0) {
            return true;
        }
        uint8_t buf[16384];
        int n = recv(mSock, (char*)buf, sizeof(buf), 0);
        if (n <= 0) {
            return false;
        }
        mReader.Append(buf, (size_t)n);
        nlohmann::json msg;
        while (mReader.Next(msg)) {
            out.push_back(std::move(msg));
        }
        return true;
    }
    void Close() override {
        if (mSock != ZMP_BAD_SOCK) {
#ifdef _WIN32
            shutdown(mSock, SD_BOTH);
#else
            shutdown(mSock, SHUT_RDWR);
#endif
            ZmpCloseSock(mSock);
            mSock = ZMP_BAD_SOCK;
        }
    }

  private:
    SockT mSock;
    FrameReader mReader;
};

DialResult DialTcp(const Address& a, int timeoutMs) {
    DialResult r;
#ifdef _WIN32
    static bool sWsa = [] {
        WSADATA d;
        return WSAStartup(MAKEWORD(2, 2), &d) == 0;
    }();
    (void)sWsa;
#endif
    addrinfo hints{};
    hints.ai_family = AF_UNSPEC; // IPv4 and IPv6
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_protocol = IPPROTO_TCP;
    addrinfo* res = nullptr;
    if (getaddrinfo(a.host.c_str(), std::to_string(a.port).c_str(), &hints, &res) != 0 || res == nullptr) {
        r.reason = "unreachable";
        r.detail = "cannot resolve " + a.host;
        return r;
    }
    auto deadline = std::chrono::steady_clock::now() + std::chrono::milliseconds(timeoutMs);
    for (addrinfo* ai = res; ai != nullptr; ai = ai->ai_next) {
        SockT s = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
        if (s == ZMP_BAD_SOCK) {
            continue;
        }
        SetBlocking(s, false);
        int rc = connect(s, ai->ai_addr, (int)ai->ai_addrlen);
        bool ok = rc == 0;
        if (!ok) {
            int left =
                (int)std::chrono::duration_cast<std::chrono::milliseconds>(deadline - std::chrono::steady_clock::now())
                    .count();
            if (left > 0) {
                fd_set wr, ex;
                FD_ZERO(&wr);
                FD_ZERO(&ex);
                FD_SET(s, &wr);
                FD_SET(s, &ex);
                timeval tv{ left / 1000, (left % 1000) * 1000 };
                if (select((int)s + 1, nullptr, &wr, &ex, &tv) > 0 && FD_ISSET(s, &wr)) {
                    int err = 0;
                    socklen_t len = sizeof(err);
                    getsockopt(s, SOL_SOCKET, SO_ERROR, (char*)&err, &len);
                    ok = err == 0;
                }
            }
        }
        if (ok) {
            SetBlocking(s, true);
            int one = 1;
            setsockopt(s, IPPROTO_TCP, TCP_NODELAY, (const char*)&one, sizeof(one)); // (lockstep: small frames, now)
            freeaddrinfo(res);
            r.conn = std::make_unique<TcpConn>(s);
            return r;
        }
        ZmpCloseSock(s);
    }
    freeaddrinfo(res);
    r.reason = "unreachable";
    r.detail = "cannot connect to " + a.Text();
    return r;
}

#ifdef _WIN32
// ---- WebSocket (WinHTTP) -------------------------------------------------------------------------------------------
std::wstring Widen(const std::string& s) {
    if (s.empty()) {
        return std::wstring();
    }
    int n = MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), nullptr, 0);
    std::wstring w((size_t)n, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), w.data(), n);
    return w;
}

class WsConn : public Conn {
  public:
    ~WsConn() override {
        Close();
    }

    bool Open(const Address& a, int timeoutMs, std::string& reason, std::string& detail) {
        bool secure = a.kind == Address::Kind::Wss;
        mSession = WinHttpOpen(L"zmp-client", WINHTTP_ACCESS_TYPE_AUTOMATIC_PROXY, WINHTTP_NO_PROXY_NAME,
                               WINHTTP_NO_PROXY_BYPASS, 0);
        if (mSession == nullptr) { // (Windows before 8.1)
            mSession = WinHttpOpen(L"zmp-client", WINHTTP_ACCESS_TYPE_DEFAULT_PROXY, WINHTTP_NO_PROXY_NAME,
                                   WINHTTP_NO_PROXY_BYPASS, 0);
        }
        if (mSession == nullptr) {
            return Fail("unsupported", "WinHttpOpen", reason, detail);
        }
        WinHttpSetTimeouts(mSession, timeoutMs, timeoutMs, 10000, 10000);
        mConnect = WinHttpConnect(mSession, Widen(a.host).c_str(), (INTERNET_PORT)a.port, 0);
        if (mConnect == nullptr) {
            return Fail("unreachable", "WinHttpConnect", reason, detail);
        }
        HINTERNET req = WinHttpOpenRequest(mConnect, L"GET", Widen(a.path).c_str(), nullptr, WINHTTP_NO_REFERER,
                                           WINHTTP_DEFAULT_ACCEPT_TYPES, secure ? WINHTTP_FLAG_SECURE : 0);
        if (req == nullptr) {
            return Fail("unreachable", "WinHttpOpenRequest", reason, detail);
        }
        // (no certificate option is relaxed: an untrusted, expired or wrong-name certificate is refused)
        bool ok = WinHttpSetOption(req, WINHTTP_OPTION_UPGRADE_TO_WEB_SOCKET, nullptr, 0) &&
                  WinHttpSendRequest(req, WINHTTP_NO_ADDITIONAL_HEADERS, 0, WINHTTP_NO_REQUEST_DATA, 0, 0, 0) &&
                  WinHttpReceiveResponse(req, nullptr);
        if (!ok) {
            DWORD err = GetLastError();
            WinHttpCloseHandle(req);
            return Fail(IsTlsError(err) ? "tls_error" : "unreachable", "handshake", reason, detail, err);
        }
        DWORD status = 0, len = sizeof(status);
        WinHttpQueryHeaders(req, WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER, WINHTTP_HEADER_NAME_BY_INDEX,
                            &status, &len, WINHTTP_NO_HEADER_INDEX);
        if (status != 101) {
            WinHttpCloseHandle(req);
            reason = "not_zmp_server";
            detail = "HTTP " + std::to_string(status) + " instead of a WebSocket upgrade";
            return false;
        }
        mWs = WinHttpWebSocketCompleteUpgrade(req, 0);
        DWORD upgradeErr = GetLastError();
        WinHttpCloseHandle(req);
        if (mWs == nullptr) {
            return Fail("not_zmp_server", "WinHttpWebSocketCompleteUpgrade", reason, detail, upgradeErr);
        }
        DWORD forever = 0; // (the client pings every second; a quiet socket is not a dead one)
        WinHttpSetOption(mWs, WINHTTP_OPTION_RECEIVE_TIMEOUT, &forever, sizeof(forever));
        mReader = std::thread(&WsConn::ReadLoop, this);
        return true;
    }

    bool SendFrame(const std::vector<uint8_t>& frame) override {
        if (mWs == nullptr || frame.size() < 4) {
            return false;
        }
        // (the length prefix is the TCP framing; a WebSocket message carries the body alone, D-239)
        return WinHttpWebSocketSend(mWs, WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE, (PVOID)(frame.data() + 4),
                                    (DWORD)(frame.size() - 4)) == NO_ERROR;
    }

    bool Poll(int ms, std::vector<nlohmann::json>& out) override {
        std::deque<std::vector<uint8_t>> got;
        bool closed;
        {
            std::unique_lock<std::mutex> lk(mMutex);
            mCv.wait_for(lk, std::chrono::milliseconds(ms), [this] { return !mIn.empty() || mClosed; });
            got.swap(mIn);
            closed = mClosed;
        }
        for (auto& body : got) {
            out.push_back(nlohmann::json::from_msgpack(body));
        }
        return !closed;
    }

    void Close() override {
        if (mWs != nullptr) {
            // (closing the handle ends the receive the reader thread is blocked in)
            WinHttpWebSocketShutdown(mWs, WINHTTP_WEB_SOCKET_SUCCESS_CLOSE_STATUS, nullptr, 0);
            WinHttpCloseHandle(mWs);
        }
        if (mReader.joinable()) {
            mReader.join();
        }
        mWs = nullptr;
        if (mConnect != nullptr) {
            WinHttpCloseHandle(mConnect);
            mConnect = nullptr;
        }
        if (mSession != nullptr) {
            WinHttpCloseHandle(mSession);
            mSession = nullptr;
        }
    }

  private:
    static bool IsTlsError(DWORD err) {
        return err == ERROR_WINHTTP_SECURE_FAILURE || err == ERROR_WINHTTP_SECURE_INVALID_CA ||
               err == ERROR_WINHTTP_SECURE_CERT_CN_INVALID || err == ERROR_WINHTTP_SECURE_CERT_DATE_INVALID ||
               err == ERROR_WINHTTP_SECURE_INVALID_CERT || err == ERROR_WINHTTP_SECURE_CERT_REVOKED ||
               err == ERROR_WINHTTP_SECURE_CHANNEL_ERROR || err == ERROR_WINHTTP_SECURE_CERT_WRONG_USAGE;
    }

    static bool Fail(const char* code, const char* step, std::string& reason, std::string& detail,
                     DWORD err = GetLastError()) {
        reason = code;
        detail = std::string(step) + " failed (WinHTTP error " + std::to_string(err) + ")";
        return false;
    }

    void ReadLoop() {
        std::vector<uint8_t> msg;
        std::vector<uint8_t> buf(65536);
        bool text = false;
        for (;;) {
            DWORD n = 0;
            WINHTTP_WEB_SOCKET_BUFFER_TYPE type;
            DWORD err = WinHttpWebSocketReceive(mWs, buf.data(), (DWORD)buf.size(), &n, &type);
            if (err != NO_ERROR || type == WINHTTP_WEB_SOCKET_CLOSE_BUFFER_TYPE) {
                break;
            }
            if (type == WINHTTP_WEB_SOCKET_UTF8_FRAGMENT_BUFFER_TYPE) {
                text = true; // (text messages are not part of the protocol: skipped, as the server does)
                continue;
            }
            if (type == WINHTTP_WEB_SOCKET_UTF8_MESSAGE_BUFFER_TYPE) {
                text = false;
                continue;
            }
            msg.insert(msg.end(), buf.begin(), buf.begin() + n);
            if (msg.size() > kMaxFrameSize) {
                Log("net: websocket message over the size limit");
                break;
            }
            if (type == WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE) {
                std::lock_guard<std::mutex> lk(mMutex);
                mIn.push_back(std::move(msg));
                msg.clear();
                mCv.notify_one();
            }
            (void)text;
        }
        std::lock_guard<std::mutex> lk(mMutex);
        mClosed = true;
        mCv.notify_one();
    }

    HINTERNET mSession = nullptr;
    HINTERNET mConnect = nullptr;
    HINTERNET mWs = nullptr;
    std::thread mReader;
    std::mutex mMutex;
    std::condition_variable mCv;
    std::deque<std::vector<uint8_t>> mIn;
    bool mClosed = false;
};
#endif

} // namespace

DialResult Dial(const Address& a, int timeoutMs) {
    if (a.kind == Address::Kind::Tcp) {
        return DialTcp(a, timeoutMs);
    }
    DialResult r;
#ifdef _WIN32
    auto ws = std::make_unique<WsConn>();
    if (ws->Open(a, timeoutMs, r.reason, r.detail)) {
        r.conn = std::move(ws);
    }
#else
    r.reason = "unsupported";
    r.detail = "WebSocket addresses need the Windows build or the browser";
#endif
    return r;
}

} // namespace Zmp

#endif
