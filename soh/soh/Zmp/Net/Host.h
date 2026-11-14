#pragma once

#include <string>

#include <nlohmann/json.hpp>

// "Alojar partida" (docs/PRODUCTO_PUBLICO.md 3.3 and "Alojar sin complicaciones", docs/PROTOCOLO_5.md 9; D-260): the
// zmp-server.exe that comes with the game runs as a child process of soh.exe (no window; it dies with the game unless
// gZmp.Host.KeepRunning), writes its state file (invite code, reachability, players) and this PC joins its own room
// at 127.0.0.1. The room is announced on the LAN list (Net/Lan.h). Desktop only; the web build cannot host.
namespace Zmp::Host {

struct Options {
    std::string scope = "lan"; // "lan": every interface and the router (UPnP); "local": 127.0.0.1 only
    int port = 47100;
    std::string room;
    std::string password; // empty: the server makes one ("lan") or none ("local")
};

struct Status {
    bool running = false;
    bool ready = false; // the state file says the server listens
    std::string error;  // why it is not running (a text for the player, Spanish/English by Text::T)
    int exitCode = 0;   // when it stopped on its own
    nlohmann::json state = nlohmann::json::object(); // the state file (docs/PROTOCOLO_5.md 9.1)
    std::string serverExe;                           // the binary used
    std::string dataDir;                             // <app data>/server
};

bool Start(const Options& opt);
void Stop();
Status Get();
// Each frame (game thread): reads the state file, joins this PC's own room when it is ready, keeps the LAN announce.
void Poll();
// The game is closing.
void Shutdown();

} // namespace Zmp::Host
