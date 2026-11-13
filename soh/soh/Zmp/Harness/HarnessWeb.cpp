// ZMP web test builds (D-242): the harness without a socket. The browser has no threads and no TCP, so the page's
// JavaScript hands commands in and collects answers through two exported functions, and the game thread processes
// them between ticks exactly as it does the ones that come through the TCP server on the desktop (HarnessServer.cpp).
//
//   Module.ccall("Zmp_HarnessWebRequest", null, ["string"], [jsonLine])   one command (same JSON as the TCP line)
//   Module.ccall("Zmp_HarnessWebPoll", "string", [], [])                   JSON array of the answers ready so far
//
// tools/harness/zmp/webinstance.py drives this over the DevTools protocol of a headless tab (webtab.py).

#if defined(ZMP_HARNESS) && defined(__EMSCRIPTEN__)

#include <string>
#include <vector>

#include <emscripten.h>
#include <nlohmann/json.hpp>

#include "HarnessQueue.h"
#include "soh/Zmp/ZmpLog.h"

namespace Zmp::Harness {

namespace {
std::vector<RequestPtr> sPending;     // handed by the page, not yet dispatched
std::vector<nlohmann::json> sReplies; // answered, not yet collected by the page
std::string sPollBuffer; // what Zmp_HarnessWebPoll returned last (its memory stays valid until the next poll)
bool sRunning = false;
} // namespace

void PushRequest(RequestPtr req) {
    sPending.push_back(std::move(req));
}

std::vector<RequestPtr> TakeRequests() {
    std::vector<RequestPtr> out;
    out.swap(sPending);
    return out;
}

void WebReply(nlohmann::json resp) {
    sReplies.push_back(std::move(resp));
}

bool Start(uint16_t port) {
    (void)port; // (there is no port: the page is the only client)
    sRunning = true;
    Log("harness: web transport ready (commands through Zmp_HarnessWebRequest/Poll)");
    return true;
}

void Stop() {
    sRunning = false;
    sPending.clear();
    sReplies.clear();
}

bool IsRunning() {
    return sRunning;
}

uint16_t GetPort() {
    return 0;
}

} // namespace Zmp::Harness

extern "C" EMSCRIPTEN_KEEPALIVE void Zmp_HarnessWebRequest(const char* line) {
    if (!Zmp::Harness::IsRunning() || line == nullptr) {
        return;
    }
    auto req = std::make_shared<Zmp::Harness::Request>();
    try {
        req->cmd = nlohmann::json::parse(line);
    } catch (const std::exception& e) {
        req->cmd = nlohmann::json::object();
        req->Reply({ { "ok", false }, { "error", std::string("bad json: ") + e.what() } });
        return;
    }
    Zmp::Harness::PushRequest(std::move(req));
}

extern "C" EMSCRIPTEN_KEEPALIVE const char* Zmp_HarnessWebPoll(void) {
    nlohmann::json arr = nlohmann::json::array();
    for (auto& r : Zmp::Harness::sReplies) {
        arr.push_back(std::move(r));
    }
    Zmp::Harness::sReplies.clear();
    Zmp::Harness::sPollBuffer = arr.dump();
    return Zmp::Harness::sPollBuffer.c_str();
}

#endif
