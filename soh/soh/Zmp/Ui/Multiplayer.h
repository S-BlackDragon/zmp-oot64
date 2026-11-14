#pragma once

#include <string>
#include <vector>

#include "soh/Zmp/Net/ZmpClient.h"

// The multiplayer screens (docs/PRODUCTO_PUBLICO.md 3.3): Join (invite code, games on this network, by address with
// password and recent servers), Host (the bundled server as a child process, its invite code and reachability),
// Profile (name, changed in session too; language) and Advanced (the room window's settings of before). Every
// button calls one of the functions below; the harness calls the same ones (tests).
namespace Zmp::Ui {

void DrawMultiplayer();
// Selects a tab on the next frame ("join", "host", "profile", "advanced").
void SelectTab(const std::string& tab);

// Join with an invite code or zmp:// link: fills address, room and password and connects. False (and the reason's
// text in *message) when the code is not good, or it carries a hint and no password was given (the field is
// focused instead).
bool JoinWithCode(const std::string& code, const std::string& password, std::string* message);
// Join by address (any form of the grammar of Net/Address.h), with an optional room password.
bool JoinAddress(const std::string& address, const std::string& room, const std::string& password,
                 std::string* message);
// The games of this network (desktop only) and joining one.
std::vector<std::string> LanRoomsText();
bool JoinLan(size_t index, const std::string& password, std::string* message);
// Another name, now (RENAME while connected; else only the setting).
bool RequestRename(const std::string& name, std::string* message);

// What the status line says of a connection, in the player's language (rejections by their code, D-259).
std::string StatusText(const NetStatus& st);

// The recent servers (newest first): "address\troom".
std::vector<std::string> RecentList();
void RecentRemember(const std::string& address, const std::string& room);

// Web (zmp-config.json / URL, D-262): what the hosting site fixed.
bool ServerLocked();

// zmp:// links (Windows): register this game for them (HKEY_CURRENT_USER, when the player asks), and join with the
// link soh.exe was opened with (Zmp_Init).
bool RegisterLinkHandler();
bool JoinFromCommandLine();
// One copy per game folder (Windows): a zmp:// link opened while the game runs is handed to it through a named pipe
// (Zmp_LinkHandOff in main, before any window) and joined on the game thread (OnFrame). Zmp_Init starts it.
void StartLinkServer();

// Each frame (game thread): applies the answers of the network (the confirmed name), the recent list, the host.
void OnFrame();

} // namespace Zmp::Ui
