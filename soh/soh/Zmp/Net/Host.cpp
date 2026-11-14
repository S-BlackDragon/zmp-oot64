#include "Host.h"

#if defined(__EMSCRIPTEN__) || !defined(_WIN32)

namespace Zmp::Host {
bool Start(const Options&) {
    return false;
}
void Stop() {
}
Status Get() {
    Status s;
    s.error = "hosting needs the Windows desktop app";
    return s;
}
void Poll() {
}
void Shutdown() {
}
} // namespace Zmp::Host

#else

#include <chrono>
#include <filesystem>
#include <fstream>
#include <mutex>
#include <sstream>

#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#include <winhttp.h>
#pragma comment(lib, "winhttp.lib")

#include <ship/Context.h>
#include <libultraship/bridge/consolevariablebridge.h>

#include "Crypto.h"
#include "Lan.h"
#include "ZmpClient.h"
#include "soh/Zmp/ZmpCVars.h"
#include "soh/Zmp/ZmpLog.h"
#include "soh/Zmp/Ui/ZmpText.h"

namespace fs = std::filesystem;

namespace Zmp::Host {

namespace {

using Clock = std::chrono::steady_clock;

struct Child {
    HANDLE process = nullptr;
    HANDLE job = nullptr;
    Options opt;
    Status status;
    fs::path stateFile;
    fs::file_time_type stateTime{};
    Clock::time_point lastRead{};
    Clock::time_point startedAt{};
    // configuration steps through the local API (docs/PROTOCOLO_5.md 9.1)
    bool configured = false;
    bool joined = false;
    std::string token;
};

Child sChild;

std::wstring Widen(const std::string& s) {
    int n = MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), nullptr, 0);
    std::wstring w((size_t)n, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), w.data(), n);
    return w;
}

std::string ExeDir() {
    wchar_t buf[4096];
    DWORD n = GetModuleFileNameW(nullptr, buf, 4096);
    return fs::path(std::wstring(buf, n)).parent_path().string();
}

std::string ComputerName() {
    wchar_t buf[256];
    DWORD n = 256;
    if (GetComputerNameW(buf, &n)) {
        std::wstring w(buf, n);
        int len = WideCharToMultiByte(CP_UTF8, 0, w.data(), (int)w.size(), nullptr, 0, nullptr, nullptr);
        std::string s((size_t)len, '\0');
        WideCharToMultiByte(CP_UTF8, 0, w.data(), (int)w.size(), s.data(), len, nullptr, nullptr);
        return s;
    }
    return "PC";
}

// One request to the server's local page (127.0.0.1 only, with its token). The answer's body, or "" on failure.
bool Api(const std::string& method, const std::string& path, const std::string& body, std::string* answer) {
    // (the page and its API listen on their own port of 127.0.0.1; the state file's "page" is
    // http://127.0.0.1:<port>/#t=<token>)
    std::string page = sChild.status.state.value("page", std::string());
    size_t colon = page.find(':', 7);
    int port = colon == std::string::npos ? 0 : atoi(page.c_str() + colon + 1);
    if (port <= 0 || sChild.token.empty()) {
        return false;
    }
    HINTERNET ses =
        WinHttpOpen(L"zmp-host", WINHTTP_ACCESS_TYPE_NO_PROXY, WINHTTP_NO_PROXY_NAME, WINHTTP_NO_PROXY_BYPASS, 0);
    if (ses == nullptr) {
        return false;
    }
    WinHttpSetTimeouts(ses, 2000, 2000, 3000, 3000);
    HINTERNET con = WinHttpConnect(ses, L"127.0.0.1", (INTERNET_PORT)port, 0);
    HINTERNET req = con == nullptr ? nullptr
                                   : WinHttpOpenRequest(con, Widen(method).c_str(), Widen(path).c_str(), nullptr,
                                                        WINHTTP_NO_REFERER, WINHTTP_DEFAULT_ACCEPT_TYPES, 0);
    bool ok = false;
    if (req != nullptr) {
        std::wstring headers = L"Content-Type: application/json\r\nX-ZMP-Token: " + Widen(sChild.token) + L"\r\n";
        ok = WinHttpSendRequest(req, headers.c_str(), (DWORD)-1L, (LPVOID)body.data(), (DWORD)body.size(),
                                (DWORD)body.size(), 0) &&
             WinHttpReceiveResponse(req, nullptr);
        DWORD code = 0, len = sizeof(code);
        if (ok) {
            WinHttpQueryHeaders(req, WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER,
                                WINHTTP_HEADER_NAME_BY_INDEX, &code, &len, WINHTTP_NO_HEADER_INDEX);
            ok = code >= 200 && code < 300;
            std::string out;
            for (;;) {
                DWORD avail = 0;
                if (!WinHttpQueryDataAvailable(req, &avail) || avail == 0) {
                    break;
                }
                std::string chunk(avail, '\0');
                DWORD got = 0;
                WinHttpReadData(req, chunk.data(), avail, &got);
                out.append(chunk.data(), got);
            }
            if (answer != nullptr) {
                *answer = out;
            }
            if (!ok) {
                Log("host: " + method + " " + path + " answered HTTP " + std::to_string(code) + ": " +
                    out.substr(0, 200));
            }
        }
        WinHttpCloseHandle(req);
    }
    if (con != nullptr) {
        WinHttpCloseHandle(con);
    }
    WinHttpCloseHandle(ses);
    return ok;
}

void ReadState(bool force) {
    auto now = Clock::now();
    if (!force && now - sChild.lastRead < std::chrono::milliseconds(300)) {
        return;
    }
    sChild.lastRead = now;
    std::error_code ec;
    auto t = fs::last_write_time(sChild.stateFile, ec);
    if (ec || (!force && t == sChild.stateTime)) {
        return;
    }
    std::ifstream f(sChild.stateFile, std::ios::binary);
    std::stringstream ss;
    ss << f.rdbuf();
    try {
        nlohmann::json j = nlohmann::json::parse(ss.str());
        if (j.is_object()) {
            sChild.stateTime = t;
            sChild.status.state = j;
            std::string page = j.value("page", std::string());
            size_t t = page.find("#t=");
            if (t != std::string::npos) {
                sChild.token = page.substr(t + 3);
            }
        }
    } catch (const std::exception&) {
        // (being written: the next read gets it; the server writes it atomically, so this is rare)
    }
}

void ChildGone() {
    DWORD code = 0;
    GetExitCodeProcess(sChild.process, &code);
    CloseHandle(sChild.process);
    sChild.process = nullptr;
    if (sChild.job != nullptr) {
        CloseHandle(sChild.job);
        sChild.job = nullptr;
    }
    sChild.status.running = false;
    sChild.status.ready = false;
    sChild.status.exitCode = (int)code;
    Lan::StopAnnounce();
    Log("host: zmp-server exited with code " + std::to_string(code));
}

} // namespace

Status Get() {
    return sChild.status;
}

bool Start(const Options& optIn) {
    if (sChild.process != nullptr) {
        return true;
    }
    Options opt = optIn;
    sChild = Child();
    sChild.opt = opt;
    std::string exe = CVarGetString(ZMP_CVAR_HOST_SERVER_EXE, "");
    if (exe.empty()) {
        exe = (fs::path(ExeDir()) / "zmp-server.exe").string();
    }
    sChild.status.serverExe = exe;
    if (!fs::exists(exe)) {
        sChild.status.error = Text::T("host.missing");
        Log("host: no server at " + exe);
        return false;
    }
    std::error_code ec;
    // (absolute: the server runs with this folder as its working directory, D-260)
    fs::path data = fs::absolute(fs::path(Ship::Context::GetPathRelativeToAppDirectory("zmp-host")), ec);
    fs::create_directories(data, ec);
    sChild.status.dataDir = data.string();
    sChild.stateFile = data / "state.json";
    fs::remove(sChild.stateFile, ec);
    // The configuration of this run, written whole (docs/PROTOCOLO_5.md 9; server/internal/hostapp/config.go): the
    // room and password the player chose, or ones made up here (a password always when others can reach the PC).
    auto word = [](int n) {
        const char* letters = "abcdefghjkmnpqrstuvwxyz23456789";
        Crypto::Bytes r = Crypto::Random((size_t)n);
        std::string w;
        for (uint8_t b : r) {
            w.push_back(letters[b % 31]);
        }
        return w;
    };
    if (opt.room.empty()) {
        opt.room = "zmp-" + word(4);
    }
    if (opt.password.empty() && opt.scope != "local") {
        opt.password = word(8);
    }
    sChild.opt = opt;
    auto toml = [](const std::string& v) {
        std::string out = "\"";
        for (char c : v) {
            if (c == '\\' || c == '"') {
                out.push_back('\\');
            }
            if ((unsigned char)c < 0x20) {
                continue;
            }
            out.push_back(c);
        }
        return out + "\"";
    };
    std::string listenAddr = (opt.scope == "local" ? "127.0.0.1:" : ":") + std::to_string(opt.port);
    {
        std::ofstream cfg(data / "zmp-server.toml", std::ios::binary | std::ios::trunc);
        cfg << "# Written by the game for \"Alojar partida\" at each start (D-260).\n"
            << "listen = " << toml(listenAddr) << "\n"
            << "data = \"data\"\n"
            << "server_name = " << toml(ComputerName()) << "\n"
            << "room = " << toml(opt.room) << "\n"
            << "password = " << toml(opt.password) << "\n"
            << "invite_secret = \"password\"\n"
            << "port_mapping = " << (opt.scope == "local" ? "false" : "true") << "\n"
            << "open_browser = false\n"
            << "ui_listen = \"127.0.0.1:0\"\n";
        // "Partidas en esta red" (D-261): the server announces its room; "solo este PC" announces nothing, except in
        // test builds that point the LAN list at 127.0.0.1 (gZmp.Lan.Address).
        std::string lanAddr = CVarGetString(ZMP_CVAR_LAN_ADDRESS, "");
        int lanPort = CVarGetInteger(ZMP_CVAR_LAN_PORT, 47100);
        cfg << "lan_announce = " << (opt.scope != "local" || !lanAddr.empty() ? "true" : "false") << "\n"
            << "lan_port = " << (lanPort > 0 && lanPort < 65536 ? lanPort : 47100) << "\n";
        if (!lanAddr.empty()) {
            cfg << "lan_address = " << toml(lanAddr) << "\n";
        }
    }
    // docs/PROTOCOLO_5.md 9: the app's own child. "local" listens on 127.0.0.1 only (no firewall question, no
    // router); "lan" on every interface, and the server asks the router for the port.
    std::string cmd = "\"" + exe + "\" -app -no-browser -config \"" + (data / "zmp-server.toml").string() +
                      "\" -state-file \"" + sChild.stateFile.string() + "\"";
    bool keep = CVarGetInteger(ZMP_CVAR_HOST_KEEP, 0) != 0;
    // Its output goes to zmp-host/zmp-server.log (no console, no window).
    SECURITY_ATTRIBUTES sa{ sizeof(sa), nullptr, TRUE };
    HANDLE logFile = CreateFileW((data / "zmp-server.log").wstring().c_str(), FILE_APPEND_DATA,
                                 FILE_SHARE_READ | FILE_SHARE_WRITE, &sa, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    // (its input is NUL: a server that fails to start and asks for Enter ends at once instead of waiting)
    HANDLE nulIn =
        CreateFileW(L"NUL", GENERIC_READ, FILE_SHARE_READ | FILE_SHARE_WRITE, &sa, OPEN_EXISTING, 0, nullptr);
    STARTUPINFOW si{};
    si.cb = sizeof(si);
    if (logFile != INVALID_HANDLE_VALUE) {
        si.dwFlags = STARTF_USESTDHANDLES;
        si.hStdOutput = logFile;
        si.hStdError = logFile;
        si.hStdInput = nulIn != INVALID_HANDLE_VALUE ? nulIn : nullptr;
    }
    PROCESS_INFORMATION pi{};
    std::wstring wcmd = Widen(cmd);
    std::wstring wdir = data.wstring();
    // (no window: CREATE_NO_WINDOW; its output goes to its own log in the data folder)
    DWORD flags = CREATE_NO_WINDOW | CREATE_SUSPENDED;
    BOOL created = CreateProcessW(nullptr, wcmd.data(), nullptr, nullptr, logFile != INVALID_HANDLE_VALUE, flags,
                                  nullptr, wdir.c_str(), &si, &pi);
    DWORD createErr = GetLastError();
    if (logFile != INVALID_HANDLE_VALUE) {
        CloseHandle(logFile);
    }
    if (nulIn != INVALID_HANDLE_VALUE) {
        CloseHandle(nulIn);
    }
    if (!created) {
        DWORD err = createErr;
        sChild.status.error =
            std::string(Text::T("host.failed"))
                .replace(std::string(Text::T("host.failed")).find("%s"), 2, "error " + std::to_string(err));
        Log("host: CreateProcess failed (" + std::to_string(err) + "): " + cmd);
        return false;
    }
    if (!keep) {
        // The server goes with the game, even when the game ends badly: a job that kills its processes when its
        // last handle (this process's) closes.
        sChild.job = CreateJobObjectW(nullptr, nullptr);
        if (sChild.job != nullptr) {
            JOBOBJECT_EXTENDED_LIMIT_INFORMATION li{};
            li.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
            SetInformationJobObject(sChild.job, JobObjectExtendedLimitInformation, &li, sizeof(li));
            if (!AssignProcessToJobObject(sChild.job, pi.hProcess)) {
                Log("host: the server could not join the job (" + std::to_string(GetLastError()) +
                    "): it is stopped on exit instead");
            }
        }
    }
    ResumeThread(pi.hThread);
    CloseHandle(pi.hThread);
    sChild.process = pi.hProcess;
    sChild.status.running = true;
    sChild.startedAt = Clock::now();
    Log("host: started zmp-server (scope " + opt.scope + ", port " + std::to_string(opt.port) + ")" +
        (keep ? " and it keeps running when the game closes" : ""));
    return true;
}

void Poll() {
    if (sChild.process == nullptr) {
        return;
    }
    if (WaitForSingleObject(sChild.process, 0) == WAIT_OBJECT_0) {
        int code = 0;
        ChildGone();
        code = sChild.status.exitCode;
        std::string t = Text::T("host.exited");
        size_t at = t.find("%d");
        if (at != std::string::npos) {
            t.replace(at, 2, std::to_string(code));
        }
        sChild.status.error = t;
        return;
    }
    ReadState(false);
    const nlohmann::json& s = sChild.status.state;
    int port = s.value("port", 0);
    if (port <= 0) {
        if (Clock::now() - sChild.startedAt > std::chrono::seconds(15) && !sChild.status.ready) {
            sChild.status.error = "the server did not write its state in 15 s";
        }
        return;
    }
    std::string room = s.value("room", std::string());
    std::string password = s.value("password", std::string());
    bool roomOk = room == sChild.opt.room;
    if (!sChild.joined && roomOk && !room.empty()) {
        sChild.joined = true;
        sChild.status.ready = true;
        ConnectRequest req;
        req.addresses = { "127.0.0.1:" + std::to_string(port) };
        req.defaultPort = (uint16_t)port;
        req.room = room;
        req.name = CVarGetString(ZMP_CVAR_NAME, "Player");
        req.password = password;
        Log("host: the server is ready (room " + room + ", port " + std::to_string(port) + "): joining it");
        Client::Get().Connect(std::move(req));
    }
    // (the LAN announce is the server's own, lan_announce in its file: it goes on when the game closes and the server
    // keeps running, and the double-click server sends the same one)
}

void Stop() {
    if (sChild.process == nullptr) {
        return;
    }
    Lan::StopAnnounce();
    NetStatus st = Client::Get().GetStatus();
    if (st.via.rfind("127.0.0.1:", 0) == 0) {
        Client::Get().Disconnect();
    }
    // The clean way first (it removes the router's rule), then the hard one.
    if (!sChild.token.empty()) {
        Api("POST", "/api/stop", "{}", nullptr);
    }
    if (WaitForSingleObject(sChild.process, 4000) != WAIT_OBJECT_0) {
        Log("host: the server did not stop in 4 s: ending it");
        TerminateProcess(sChild.process, 1);
        WaitForSingleObject(sChild.process, 2000);
    }
    ChildGone();
    sChild.status.error.clear();
}

void Shutdown() {
    if (sChild.process == nullptr) {
        return;
    }
    if (CVarGetInteger(ZMP_CVAR_HOST_KEEP, 0) != 0) {
        Log("host: the game closes and the server keeps running (gZmp.Host.KeepRunning)");
        Lan::StopAnnounce();
        CloseHandle(sChild.process);
        sChild.process = nullptr;
        return;
    }
    Stop();
}

} // namespace Zmp::Host

#endif
