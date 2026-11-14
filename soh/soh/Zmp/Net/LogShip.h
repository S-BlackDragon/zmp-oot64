#pragma once

#include <cstdint>
#include <string>

#include <nlohmann/json.hpp>

// ZMP (D-247): this client's logs go to the server it plays on, so a failure on any PC can be read there afterwards.
// What is sent: logs/zmp.log (the ZMP lines) and the game's log of this run of the game, as appends (only the bytes
// the server does not have yet), plus the dump of a desync. When: every few minutes while connected, at the end of a
// session, on a desync, on a crash (from the crash handler, and again on the next start for what did not go out:
// the previous run's log) . Only to a server whose WELCOME says log_collect, with the token of that WELCOME, and
// never with the setting gZmp.Logs.Upload at 0. The uploads run on their own thread (on the web, the browser's fetch):
// the game thread never waits for the network. A failed upload is written to the local log and tried again later.
// Nothing here is read by the simulation.
namespace Zmp::LogShip {

// Zmp_Init, before the first ZMP log line of this run: names the run, notes where this run starts in each log and
// what the previous run left unsent, and starts the uploader.
void Start();
// Zmp_Deinit: the last upload of the session (bounded wait) and the mark that this run ended cleanly.
void Stop();
// The server's WELCOME (any transport): host and port as the client connected to them.
void OnWelcome(bool collect, const std::string& token, const std::string& host, uint16_t port);
// The connection that had a WELCOME is over (the session ended, or it was lost): its last upload.
void OnSessionEnd();
// A RESYNC: the logs and, if this client wrote one, the dump of that tick.
void OnDesync(uint32_t badTick);
// The crash handler: marks the run as crashed and tries to send what is unsent, a few seconds at most.
void OnCrash();
// Web: once per frame (the uploads are started and finished on the main thread).
void Poll();
// For the harness (net.logship) and the room window.
nlohmann::json Status();

} // namespace Zmp::LogShip
