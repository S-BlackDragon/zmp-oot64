#pragma once

#ifdef ZMP_HARNESS

#include <future>
#include <memory>
#include <vector>

#include <nlohmann/json.hpp>

namespace Zmp::Harness {

// One command received from a harness client. The socket thread waits on `promise`; the
// game thread fulfils it (immediately or when a wait condition is met).
struct Request {
    nlohmann::json cmd;
    std::promise<nlohmann::json> promise;
    bool answered = false;

    void Reply(nlohmann::json resp);
};

using RequestPtr = std::shared_ptr<Request>;

// Socket thread -> game thread (web: page -> game, same thread).
void PushRequest(RequestPtr req);
std::vector<RequestPtr> TakeRequests();

#ifdef __EMSCRIPTEN__
// Web (D-242): an answer waits in a list the page polls (HarnessWeb.cpp).
void WebReply(nlohmann::json resp);
#endif

inline void Request::Reply(nlohmann::json resp) {
    if (answered) {
        return;
    }
    answered = true;
    if (cmd.contains("id")) {
        resp["id"] = cmd["id"];
    }
#ifdef __EMSCRIPTEN__
    WebReply(std::move(resp));
#else
    promise.set_value(std::move(resp));
#endif
}

// Game thread: executes one command (defined in Harness.cpp).
void Dispatch(const RequestPtr& req);

} // namespace Zmp::Harness

#endif
