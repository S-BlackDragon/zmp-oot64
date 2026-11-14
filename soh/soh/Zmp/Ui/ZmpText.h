#pragma once

#include <map>
#include <string>

// The texts of ZMP's multiplayer screens in Spanish and English (docs/PRODUCTO_PUBLICO.md I6), and the player's own
// message for every rejection code of the server (docs/PROTOCOLO_5.md 6) and of this client. Language: the CVar
// gZmp.Language ("auto": the system's / the browser's). Presentation only.
namespace Zmp::Text {

// "es" or "en".
std::string Language();
// The text of a key in the current language (the key itself when unknown).
const char* T(const char* key);
// What the player reads when the server (or this client) refused the connection: the code's own text with its
// arguments; `detail` only for a code this client does not know.
std::string Rejection(const std::string& reason, const std::map<std::string, std::string>& args,
                      const std::string& detail);
// The text of an invite code error ("checksum", ...).
std::string InviteError(const std::string& error);

} // namespace Zmp::Text
