#pragma once

// "Partidas en esta red": the LAN list (docs/PRODUCTO_PUBLICO.md I5, D-261). A desktop that hosts announces its room
// every 2 s by UDP (multicast group 239.255.47.100 and broadcast, port 47100), as one JSON datagram:
//   {"zmp":1, "server_name":"PC-Saria", "port":47100, "proto":5,
//    "rooms":[{"name":"bosque", "players":2, "max":6, "engine":"native", "build_hash":"...", "password":true}]}
// The list listens on the same port and keeps what it heard in the last 6 s; the address to join is the sender's IP
// with the announced port. LAN only: nothing goes beyond the local network (multicast TTL 1). The web build has no UDP.
// Test builds may give gZmp.Lan.Address ("127.0.0.1"): the list listens there and the announce goes there only.

#include <cstdint>
#include <string>
#include <vector>

namespace Zmp::Lan {

struct Room {
    std::string address; // "ip:port", ready for Connect
    std::string serverName;
    std::string room;
    int players = 0;
    int max = 6;
    int proto = 0;
    std::string engine;
    std::string buildHash;
    bool password = false;
    double ageSeconds = 0;
};

// The list (starts listening on first use; StopListening when its screen is closed).
std::vector<Room> Rooms();
void StopListening();
bool Listening();
// The announce of this PC's hosted room (empty room: stop).
void SetAnnounce(const std::string& serverName, uint16_t port, const std::string& room, int players, bool password,
                 const std::string& buildHash);
void StopAnnounce();
// Everything stops (game exit).
void Shutdown();

} // namespace Zmp::Lan
