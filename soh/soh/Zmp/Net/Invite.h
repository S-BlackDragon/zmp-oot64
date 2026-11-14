#pragma once

#include <cstdint>
#include <string>
#include <vector>

// The invite code of docs/PROTOCOLO_5.md section 7 (version 1): where to connect (one to four addresses in the order
// they are tried, and a port), the room and the password or a hint. Same format as server/internal/invite (Go); the
// harness checks this code against its vectors (server/internal/invite/testdata/vectors.json).
namespace Zmp::Invite {

struct Code {
    std::vector<std::string> addresses; // IPv4, IPv6 (no brackets), host names, ws:// or wss:// URLs
    int port = 47100;
    std::string room;
    std::string password;
    std::string hint;
};

// Error codes: "empty", "character", "length", "checksum", "version", "malformed", "invalid".
bool Parse(const std::string& text, Code& out, std::string& error);
// "ZMP-XXXX-..." and "zmp://join/XXXX..."; empty strings when the fields cannot be encoded (error says why).
bool Format(const Code& code, std::string& text, std::string& link, std::string& error);
// The text a person typed or pasted, as the code's characters (prefixes, dashes, spaces gone; O->0, I/L->1).
std::string Normalize(const std::string& text);
// Whether the text looks like an invite code or link rather than a server address.
bool LooksLikeCode(const std::string& text);
// The address of the grammar of Address.h for one of the code's addresses ("host:port", "[v6]:port" or the URL).
std::string AddressFor(const Code& code, size_t index);

} // namespace Zmp::Invite
