#include "Multiplayer.h"

#include <algorithm>
#include <cstring>
#include <ctime>
#include <mutex>
#include <thread>
#include <vector>

#include <imgui.h>
#include <nlohmann/json.hpp>
#include <ship/Context.h>
#include <ship/window/Window.h>
#include <libultraship/bridge/consolevariablebridge.h>

#include "ZmpText.h"
#include "ZmpWindow.h"
#include "ChatBox.h"
#include "soh/SohGui/UIWidgets.hpp"
#include "soh/Zmp/ZmpCVars.h"
#include "soh/Zmp/ZmpLog.h"
#include "soh/Zmp/Net/Address.h"
#include "soh/Zmp/Net/Crypto.h"
#include "soh/Zmp/Net/Host.h"
#include "soh/Zmp/Net/Invite.h"
#include "soh/Zmp/Net/Lan.h"

#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#include <shellapi.h>
#pragma comment(lib, "shell32.lib")
#pragma comment(lib, "advapi32.lib")
extern "C" void Zmp_InstallBootLog(void);
extern "C" void Zmp_BootMark(const char* what);
#endif

namespace Zmp::Ui {

using Text::T;

namespace {

std::string sTabRequest;
char sCode[512] = "";
std::string sCodeMessage;
bool sCodeBad = false;
std::string sHint;
// The room password: typed here or carried by a code; kept in memory only (never in the config or a log).
char sPassword[72] = "";
char sServerPassword[72] = "";
char sNewName[40] = "";
bool sNewNameInit = false;
std::string sProfileMessage;
std::string sJoinMessage;
uint32_t sSeenRenameSeq = 0;
int sSeenState = -1;
std::vector<Lan::Room> sLanShown;
// Host tab
char sHostRoom[72] = "";
char sHostPassword[72] = "";
std::string sCopiedAt;
double sCopiedTime = 0;

void SaveSoon() {
    Ship::Context::GetRawInstance()->GetWindow()->GetGui()->SaveConsoleVariablesNextFrame();
}

std::string Fmt(const char* fmt, const std::string& a, const std::string& b = "", int n = -1) {
    std::string out;
    int k = 0;
    for (const char* p = fmt; *p; p++) {
        if (p[0] == '%' && p[1] == 's') {
            out += k++ == 0 ? a : b;
            p++;
        } else if (p[0] == '%' && p[1] == 'd') {
            out += std::to_string(n);
            p++;
        } else {
            out.push_back(*p);
        }
    }
    return out;
}

std::string PasswordPolicy() {
    std::string p = CVarGetString(ZMP_CVAR_WEB_PASSWORD_POLICY, "optional");
    return p == "none" || p == "required" ? p : "optional";
}

bool Busy(const NetStatus& st) {
    return st.state == NetState::Connected || st.state == NetState::Connecting || st.state == NetState::Error;
}

ImVec4 StateColor(NetState s) {
    switch (s) {
        case NetState::Connected:
            return ImVec4(0.55f, 1.0f, 0.55f, 1.0f);
        case NetState::Connecting:
            return ImVec4(1.0f, 0.9f, 0.4f, 1.0f);
        case NetState::Rejected:
        case NetState::Error:
            return ImVec4(1.0f, 0.45f, 0.45f, 1.0f);
        default:
            return ImVec4(0.8f, 0.8f, 0.8f, 1.0f);
    }
}

bool DoConnect(std::vector<std::string> addresses, const std::string& room, const std::string& password,
               std::string* message) {
    std::string name = CVarGetString(ZMP_CVAR_NAME, "Player");
    if (addresses.empty() || room.empty() || name.empty()) {
        if (message) {
            *message = T("join.badaddress");
        }
        return false;
    }
    if (PasswordPolicy() == "required" && password.empty()) {
        if (message) {
            *message = T("join.password.needed");
        }
        return false;
    }
    for (auto& a : addresses) {
        if (!ParseAddress(a, (uint16_t)CVarGetInteger(ZMP_CVAR_SERVER_PORT, ZMP_DEFAULT_PORT)).ok) {
            if (message) {
                *message = T("join.badaddress");
            }
            return false;
        }
    }
    ConnectRequest req;
    req.addresses = addresses;
    req.defaultPort = (uint16_t)CVarGetInteger(ZMP_CVAR_SERVER_PORT, ZMP_DEFAULT_PORT);
    req.room = room;
    req.name = name;
    req.password = password;
    req.serverPassword = sServerPassword;
    if (!ServerLocked()) {
        CVarSetString(ZMP_CVAR_SERVER_HOST, addresses[0].c_str());
    }
    CVarSetString(ZMP_CVAR_ROOM, room.c_str());
    SaveSoon();
    Log("ui: connect to " + addresses[0] +
        (addresses.size() > 1 ? " (+" + std::to_string(addresses.size() - 1) + ")" : "") + " room=" + room +
        (password.empty() ? "" : " (with password)"));
    Client::Get().Connect(std::move(req));
    return true;
}

nlohmann::json RecentJson() {
    try {
        nlohmann::json j = nlohmann::json::parse(CVarGetString(ZMP_CVAR_RECENT, "[]"));
        if (j.is_array()) {
            return j;
        }
    } catch (const std::exception&) {}
    return nlohmann::json::array();
}

void SetRecent(const nlohmann::json& j) {
    CVarSetString(ZMP_CVAR_RECENT, j.dump().c_str());
    SaveSoon();
}

void DrawStatus() {
    NetStatus st = Client::Get().GetStatus();
    ImGui::TextColored(StateColor(st.state), "%s", StatusText(st).c_str());
    if (st.state == NetState::Connected && st.roomPassword) {
        ImGui::SameLine();
        ImGui::TextDisabled(Text::Language() == "es" ? "(sala con contrasena)" : "(password room)");
    }
}

void DrawJoin() {
    NetStatus st = Client::Get().GetStatus();
    bool busy = Busy(st);
    bool web =
#ifdef __EMSCRIPTEN__
        true;
#else
        false;
#endif
    if (busy) {
        // In a room (or on the way): what matters is where, who is there, and the way out.
        DrawStatus();
        if (ImGui::Button(T("join.disconnect"))) {
            Client::Get().Disconnect();
            sJoinMessage.clear();
        }
        ImGui::Separator();
        RoomWindow::DrawRoomPlayers();
        return;
    }

    // 1. Invite code
    ImGui::SeparatorText(T("join.code"));
    ImGui::BeginDisabled(busy);
    ImGui::TextDisabled("%s", T("join.code.hint"));
    ImGui::SetNextItemWidth(-1);
    ImGui::InputText("##ZmpCode", sCode, sizeof(sCode));
    if (!sHint.empty()) {
        ImGui::TextWrapped("%s", Fmt(T("join.code.hintpw"), sHint).c_str());
    }
    if (ImGui::Button(T("join.code.button"))) {
        JoinWithCode(sCode, sPassword, &sCodeMessage);
    }
    ImGui::EndDisabled();
    if (!sCodeMessage.empty()) {
        ImGui::TextColored(sCodeBad ? ImVec4(1.0f, 0.5f, 0.45f, 1.0f) : ImVec4(0.6f, 1.0f, 0.6f, 1.0f), "%s",
                           sCodeMessage.c_str());
    }

    // 2. This network (desktop)
    if (!web) {
        ImGui::SeparatorText(T("join.lan"));
        sLanShown = Lan::Rooms();
        if (sLanShown.empty()) {
            ImGui::TextDisabled("%s", T("join.lan.none"));
        } else if (ImGui::BeginTable("##ZmpLan", 5, ImGuiTableFlags_Borders | ImGuiTableFlags_RowBg)) {
            bool es = Text::Language() == "es";
            ImGui::TableSetupColumn(T("join.room"));
            ImGui::TableSetupColumn(es ? "Anfitrion" : "Host");
            ImGui::TableSetupColumn(es ? "Jugadores" : "Players");
            ImGui::TableSetupColumn(es ? "Clave" : "Password");
            ImGui::TableSetupColumn("");
            ImGui::TableHeadersRow();
            for (size_t i = 0; i < sLanShown.size(); i++) {
                auto& r = sLanShown[i];
                ImGui::TableNextRow();
                ImGui::TableNextColumn();
                ImGui::Text("%s", r.room.c_str());
                ImGui::TableNextColumn();
                ImGui::Text("%s (%s)", r.serverName.c_str(), r.address.c_str());
                ImGui::TableNextColumn();
                ImGui::Text("%d/%d", r.players, r.max);
                ImGui::TableNextColumn();
                ImGui::Text("%s", r.password ? (es ? "si" : "yes") : "no");
                ImGui::TableNextColumn();
                ImGui::BeginDisabled(busy);
                ImGui::PushID((int)i);
                if (ImGui::Button(T("join.lan.enter"))) {
                    JoinLan(i, sPassword, &sJoinMessage);
                }
                ImGui::PopID();
                ImGui::EndDisabled();
            }
            ImGui::EndTable();
        }
    }

    // 3. By address
    ImGui::SeparatorText(T("join.address"));
    std::string host = CVarGetString(ZMP_CVAR_SERVER_HOST, ZMP_DEFAULT_HOST);
    std::string room = CVarGetString(ZMP_CVAR_ROOM, ZMP_DEFAULT_ROOM);
    bool locked = ServerLocked();
    ImGui::Text("%s%s", T("join.server"), locked ? (std::string(" ") + T("join.server.locked")).c_str() : "");
    ImGui::BeginDisabled(busy || locked);
    ImGui::SetNextItemWidth(-1);
    if (UIWidgets::InputString("##ZmpHost", &host)) {
        CVarSetString(ZMP_CVAR_SERVER_HOST, host.c_str());
        SaveSoon();
    }
    ImGui::EndDisabled();
    ImGui::TextDisabled("%s", T("join.server.help"));
    ImGui::BeginDisabled(busy);
    ImGui::Text("%s", T("join.room"));
    ImGui::SetNextItemWidth(-1);
    if (UIWidgets::InputString("##ZmpRoom", &room)) {
        CVarSetString(ZMP_CVAR_ROOM, room.c_str());
        SaveSoon();
    }
    std::string policy = PasswordPolicy();
    if (policy != "none") {
        ImGui::Text("%s", T(policy == "required" ? "join.password.required" : "join.password"));
        ImGui::SetNextItemWidth(-1);
        ImGui::InputText("##ZmpPassword", sPassword, sizeof(sPassword), ImGuiInputTextFlags_Password);
    }
    ImGui::EndDisabled();

    // recent servers
    nlohmann::json recent = RecentJson();
    if (!locked) {
        ImGui::Text("%s:", T("join.recent"));
        if (recent.empty()) {
            ImGui::SameLine();
            ImGui::TextDisabled("%s", T("join.recent.none"));
        }
        int remove = -1;
        for (size_t i = 0; i < recent.size() && i < 8; i++) {
            std::string a = recent[i].value("address", std::string());
            std::string r = recent[i].value("room", std::string());
            ImGui::PushID((int)i + 100);
            ImGui::BeginDisabled(busy);
            if (ImGui::SmallButton((a + " / " + r).c_str())) {
                CVarSetString(ZMP_CVAR_SERVER_HOST, a.c_str());
                CVarSetString(ZMP_CVAR_ROOM, r.c_str());
                SaveSoon();
            }
            ImGui::EndDisabled();
            ImGui::SameLine();
            if (ImGui::SmallButton("x")) {
                remove = (int)i;
            }
            ImGui::PopID();
        }
        if (remove >= 0) {
            recent.erase(recent.begin() + remove);
            SetRecent(recent);
        }
    }

    if (!busy) {
        if (ImGui::Button(T("join.connect"))) {
            JoinAddress(host, room, sPassword, &sJoinMessage);
        }
    } else if (ImGui::Button(T("join.disconnect"))) {
        Client::Get().Disconnect();
        sJoinMessage.clear();
    }
    if (!sJoinMessage.empty() && !busy) {
        ImGui::TextColored(ImVec4(1.0f, 0.5f, 0.45f, 1.0f), "%s", sJoinMessage.c_str());
    }
    ImGui::Separator();
    DrawStatus();
    RoomWindow::DrawRoomPlayers();
}

void CopyButton(const char* label, const std::string& text) {
    if (ImGui::Button(label)) {
        ImGui::SetClipboardText(text.c_str());
        sCopiedAt = label;
        sCopiedTime = ImGui::GetTime();
    }
    if (sCopiedAt == label && ImGui::GetTime() - sCopiedTime < 2.0) {
        ImGui::SameLine();
        ImGui::TextColored(ImVec4(0.6f, 1.0f, 0.6f, 1.0f), "%s", T("host.copied"));
    }
}

void DrawHost() {
#ifdef __EMSCRIPTEN__
    ImGui::TextWrapped("%s", T("host.web"));
#else
    bool es = Text::Language() == "es";
    Host::Status hs = Host::Get();
    ImGui::SeparatorText(T("host.title"));
    if (!hs.running) {
        std::string scope = CVarGetString(ZMP_CVAR_HOST_SCOPE, "lan");
        int port = CVarGetInteger(ZMP_CVAR_HOST_PORT, 47100);
        if (sHostRoom[0] == '\0') {
            snprintf(sHostRoom, sizeof(sHostRoom), "%s", CVarGetString(ZMP_CVAR_HOST_ROOM, ""));
        }
        ImGui::Text("%s", T("host.scope"));
        if (ImGui::RadioButton(T("host.scope.lanshort"), scope == "lan")) {
            CVarSetString(ZMP_CVAR_HOST_SCOPE, "lan");
            SaveSoon();
        }
        ImGui::Indent();
        ImGui::PushStyleColor(ImGuiCol_Text, ImGui::GetStyle().Colors[ImGuiCol_TextDisabled]);
        ImGui::TextWrapped("%s", T("host.scope.lanhelp"));
        ImGui::PopStyleColor();
        ImGui::Unindent();
        if (ImGui::RadioButton(T("host.scope.local"), scope == "local")) {
            CVarSetString(ZMP_CVAR_HOST_SCOPE, "local");
            SaveSoon();
        }
        ImGui::Text("%s %s", T("host.room"), es ? "(vacia: una al azar)" : "(empty: a random one)");
        ImGui::SetNextItemWidth(-1);
        if (ImGui::InputText("##ZmpHostRoom", sHostRoom, sizeof(sHostRoom))) {
            CVarSetString(ZMP_CVAR_HOST_ROOM, sHostRoom);
            SaveSoon();
        }
        ImGui::Text("%s %s", T("host.password"), es ? "(vacia: una al azar)" : "(empty: a random one)");
        ImGui::SetNextItemWidth(-1);
        ImGui::InputText("##ZmpHostPassword", sHostPassword, sizeof(sHostPassword), ImGuiInputTextFlags_Password);
        ImGui::Text("%s", T("host.port"));
        ImGui::SetNextItemWidth(120);
        if (ImGui::InputInt("##ZmpHostPort", &port)) {
            CVarSetInteger(ZMP_CVAR_HOST_PORT, std::clamp(port, 1024, 65535));
            SaveSoon();
        }
        bool keep = CVarGetInteger(ZMP_CVAR_HOST_KEEP, 0) != 0;
        if (ImGui::Checkbox(es ? "Seguir alojando al cerrar el juego" : "Keep hosting after the game closes", &keep)) {
            CVarSetInteger(ZMP_CVAR_HOST_KEEP, keep ? 1 : 0);
            SaveSoon();
        }
        if (ImGui::Button(T("host.start"))) {
            Host::Options o;
            o.scope = scope;
            o.port = port;
            o.room = sHostRoom;
            o.password = sHostPassword;
            Host::Start(o);
        }
        if (!hs.error.empty()) {
            ImGui::TextColored(ImVec4(1.0f, 0.5f, 0.45f, 1.0f), "%s", hs.error.c_str());
        }
        return;
    }
    const nlohmann::json& s = hs.state;
    if (!hs.ready) {
        ImGui::Text("%s", T("host.starting"));
    } else {
        ImGui::Text("%s", Fmt(T("host.running"), s.value("room", std::string()), "", s.value("port", 0)).c_str());
        std::string pw = s.value("password", std::string());
        ImGui::Text("%s: %s", T("host.password"), pw.empty() ? T("host.nopassword") : pw.c_str());
        if (s.contains("invite") && s["invite"].is_object()) {
            std::string code = s["invite"].value("code", std::string());
            std::string link = s["invite"].value("link", std::string());
            ImGui::SeparatorText(T("host.code"));
            ImGui::TextWrapped("%s", code.c_str());
            CopyButton(T("host.copy"), code);
            ImGui::SameLine();
            CopyButton(T("host.copylink"), link);
        }
        if (s.contains("reach") && s["reach"].is_object()) {
            const auto& r = s["reach"];
            ImGui::SeparatorText(T("host.reach"));
            std::string st = r.value("state", std::string());
            ImVec4 c = st == "open" || st == "confirmed"        ? ImVec4(0.6f, 1.0f, 0.6f, 1.0f)
                       : st == "checking"                       ? ImVec4(1.0f, 0.9f, 0.4f, 1.0f)
                       : st == "local_only" || st == "disabled" ? ImVec4(0.8f, 0.8f, 0.8f, 1.0f)
                                                                : ImVec4(1.0f, 0.6f, 0.4f, 1.0f);
            ImGui::TextColored(c, "%s", r.value(es ? "message_es" : "message_en", st).c_str());
            std::string advice = r.value(es ? "advice_es" : "advice_en", std::string());
            if (!advice.empty()) {
                ImGui::TextWrapped("%s", advice.c_str());
            }
            if (r.contains("lan") && r["lan"].is_array() && !r["lan"].empty() && r["lan"][0].is_string()) {
                std::string url =
                    "http://" + r["lan"][0].get<std::string>() + ":" + std::to_string(s.value("port", 0)) + "/";
                ImGui::TextDisabled("%s", Fmt(T("host.lanbrowser"), url).c_str());
            }
        }
        if (s.contains("players") && s["players"].is_array()) {
            ImGui::Text("%s: %d", es ? "Jugadores" : "Players", (int)s["players"].size());
        }
    }
    if (ImGui::Button(T("host.stop"))) {
        Host::Stop();
    }
#endif
}

void DrawProfile() {
    NetStatus st = Client::Get().GetStatus();
    if (!sNewNameInit) {
        snprintf(sNewName, sizeof(sNewName), "%s", CVarGetString(ZMP_CVAR_NAME, "Player"));
        sNewNameInit = true;
    }
    bool locked = CVarGetInteger(ZMP_CVAR_WEB_LOCK_NAME, 0) != 0;
    ImGui::SeparatorText(T("profile.name"));
    ImGui::PushStyleColor(ImGuiCol_Text, ImGui::GetStyle().Colors[ImGuiCol_TextDisabled]);
    ImGui::TextWrapped("%s", T("profile.name.help"));
    ImGui::PopStyleColor();
    ImGui::BeginDisabled(locked);
    ImGui::SetNextItemWidth(240);
    ImGui::InputText("##ZmpNewName", sNewName, sizeof(sNewName));
    ImGui::SameLine();
    if (ImGui::Button(T("profile.rename"))) {
        RequestRename(sNewName, &sProfileMessage);
    }
    ImGui::EndDisabled();
    if (!sProfileMessage.empty()) {
        ImGui::Text("%s", sProfileMessage.c_str());
    }
    ImGui::SeparatorText(T("profile.language"));
    std::string lang = CVarGetString(ZMP_CVAR_LANGUAGE, "auto");
    const char* names[] = { "Auto", "Espanol", "English" };
    const char* codes[] = { "auto", "es", "en" };
    for (int i = 0; i < 3; i++) {
        if (i > 0) {
            ImGui::SameLine();
        }
        if (ImGui::RadioButton(names[i], lang == codes[i])) {
            CVarSetString(ZMP_CVAR_LANGUAGE, codes[i]);
            SaveSoon();
        }
    }
    (void)st;
}

void DrawAdvanced() {
    bool es = Text::Language() == "es";
    NetStatus st = Client::Get().GetStatus();
    ImGui::BeginDisabled(Busy(st));
    int port = CVarGetInteger(ZMP_CVAR_SERVER_PORT, ZMP_DEFAULT_PORT);
    ImGui::Text("%s", es ? "Puerto si la direccion no lo dice" : "Port when the address has none");
    ImGui::SetNextItemWidth(120);
    if (ImGui::InputInt("##ZmpPort", &port)) {
        CVarSetInteger(ZMP_CVAR_SERVER_PORT, std::clamp(port, 1, 65535));
        SaveSoon();
    }
    ImGui::Text("%s", es ? "Contrasena del servidor (si tiene)" : "Server password (if it has one)");
    ImGui::SetNextItemWidth(-1);
    ImGui::InputText("##ZmpServerPassword", sServerPassword, sizeof(sServerPassword), ImGuiInputTextFlags_Password);
    ImGui::EndDisabled();
    ImGui::Separator();
#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
    ImGui::Separator();
    static std::string sLinkMessage;
    if (ImGui::Button(es ? "Abrir los enlaces zmp:// con este juego" : "Open zmp:// links with this game")) {
        sLinkMessage = RegisterLinkHandler()
                           ? (es ? "Hecho: un enlace zmp:// abre este juego y entra en la sala"
                                 : "Done: a zmp:// link opens this game and joins the room")
                           : (es ? "No se pudo registrar (mira el registro)" : "Could not register it (see the log)");
    }
    if (!sLinkMessage.empty()) {
        ImGui::SameLine();
        ImGui::TextDisabled("%s", sLinkMessage.c_str());
    }
#endif
    ImGui::Separator();
    RoomWindow::DrawAdvancedSettings();
}

} // namespace

void SelectTab(const std::string& tab) {
    sTabRequest = tab;
}

bool ServerLocked() {
    return CVarGetInteger(ZMP_CVAR_WEB_LOCK_SERVER, 0) != 0 &&
           std::string(CVarGetString(ZMP_CVAR_SERVER_HOST, "")).size() > 0;
}

std::string StatusText(const NetStatus& st) {
    std::string where = st.via.empty() ? st.host : st.via;
    switch (st.state) {
        case NetState::Connected:
            return Fmt(T("status.connected"), where, st.room, (int)st.players.size());
        case NetState::Connecting:
            return Fmt(T("status.connecting"), where);
        case NetState::Rejected:
        case NetState::Error: {
            std::map<std::string, std::string> args = st.args;
            if (args.find("address") == args.end()) {
                args["address"] = where;
            }
            return Text::Rejection(st.reason, args, st.detail);
        }
        case NetState::Disconnected:
        default:
            return T("status.disconnected");
    }
}

bool JoinWithCode(const std::string& code, const std::string& password, std::string* message) {
    Invite::Code c;
    std::string err;
    sHint.clear();
    if (!Invite::Parse(code, c, err)) {
        sCodeBad = true;
        if (message) {
            *message = Text::InviteError(err);
        }
        Log("ui: invite code refused (" + err + ")");
        return false;
    }
    std::vector<std::string> addresses;
    for (size_t i = 0; i < c.addresses.size(); i++) {
        addresses.push_back(Invite::AddressFor(c, i));
    }
    std::string pw = c.password.empty() ? password : c.password;
    snprintf(sPassword, sizeof(sPassword), "%s", pw.c_str());
    sCodeBad = false;
    if (message) {
        *message = Fmt(T("join.code.ok"), c.room, addresses[0]);
    }
    if (!c.hint.empty() && pw.empty()) {
        sHint = c.hint; // (the password is asked for, with the host's hint)
        sCodeBad = true;
        if (message) {
            *message = Fmt(T("join.code.hintpw"), c.hint);
        }
        return false;
    }
    std::string err2;
    bool ok = DoConnect(addresses, c.room, pw, &err2);
    if (!ok && message) {
        *message = err2;
    }
    return ok;
}

bool JoinAddress(const std::string& address, const std::string& room, const std::string& password,
                 std::string* message) {
    if (Invite::LooksLikeCode(address)) {
        return JoinWithCode(address, password, message); // (a code pasted into the address field)
    }
    snprintf(sPassword, sizeof(sPassword), "%s", password.c_str());
    return DoConnect({ address }, room, password, message);
}

std::vector<std::string> LanRoomsText() {
    std::vector<std::string> out;
    sLanShown = Lan::Rooms();
    for (auto& r : sLanShown) {
        out.push_back(r.room + "\t" + r.serverName + "\t" + r.address + "\t" + std::to_string(r.players) + "/" +
                      std::to_string(r.max) + "\t" + (r.password ? "password" : "open"));
    }
    return out;
}

bool JoinLan(size_t index, const std::string& password, std::string* message) {
    if (index >= sLanShown.size()) {
        return false;
    }
    Lan::Room r = sLanShown[index];
    return DoConnect({ r.address }, r.room, password, message);
}

bool RequestRename(const std::string& nameIn, std::string* message) {
    std::string name = nameIn;
    if (name.empty() || name.size() > 32) {
        if (message) {
            *message = Text::Rejection("bad_name", {}, "");
        }
        return false;
    }
    snprintf(sNewName, sizeof(sNewName), "%s", name.c_str());
    sNewNameInit = true;
    NetStatus st = Client::Get().GetStatus();
    if (st.state == NetState::Connected && st.proto >= 5) {
        if (message) {
            *message = "...";
        }
        return Client::Get().Rename(name); // (the setting changes when the server says yes: OnFrame)
    }
    CVarSetString(ZMP_CVAR_NAME, name.c_str());
    SaveSoon();
    if (message) {
        *message = T("profile.renamed");
    }
    return true;
}

std::vector<std::string> RecentList() {
    std::vector<std::string> out;
    for (auto& e : RecentJson()) {
        out.push_back(e.value("address", std::string()) + "\t" + e.value("room", std::string()));
    }
    return out;
}

void RecentRemember(const std::string& address, const std::string& room) {
    if (address.empty()) {
        return;
    }
    nlohmann::json j = RecentJson();
    nlohmann::json out = nlohmann::json::array();
    out.push_back({ { "address", address }, { "room", room }, { "t", (int64_t)time(nullptr) } });
    for (auto& e : j) {
        if (e.value("address", std::string()) == address && e.value("room", std::string()) == room) {
            continue;
        }
        if (out.size() < 8) {
            out.push_back(e);
        }
    }
    SetRecent(out);
}

bool RegisterLinkHandler() {
#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
    // HKEY_CURRENT_USER\Software\Classes\zmp: this user only, no administrator; only when the player asks for it.
    wchar_t exe[4096];
    DWORD n = GetModuleFileNameW(nullptr, exe, 4096);
    if (n == 0 || n >= 4096) {
        return false;
    }
    std::wstring command = L"\"" + std::wstring(exe, n) + L"\" \"%1\"";
    HKEY key;
    bool ok = RegCreateKeyExW(HKEY_CURRENT_USER, L"Software\\Classes\\zmp", 0, nullptr, 0, KEY_WRITE, nullptr, &key,
                              nullptr) == ERROR_SUCCESS;
    if (ok) {
        const wchar_t* desc = L"URL:ZMP invite";
        RegSetValueExW(key, nullptr, 0, REG_SZ, (const BYTE*)desc, (DWORD)((wcslen(desc) + 1) * sizeof(wchar_t)));
        RegSetValueExW(key, L"URL Protocol", 0, REG_SZ, (const BYTE*)L"", sizeof(wchar_t));
        HKEY cmd;
        ok = RegCreateKeyExW(key, L"shell\\open\\command", 0, nullptr, 0, KEY_WRITE, nullptr, &cmd, nullptr) ==
             ERROR_SUCCESS;
        if (ok) {
            ok = RegSetValueExW(cmd, nullptr, 0, REG_SZ, (const BYTE*)command.c_str(),
                                (DWORD)((command.size() + 1) * sizeof(wchar_t))) == ERROR_SUCCESS;
            RegCloseKey(cmd);
        }
        RegCloseKey(key);
    }
    Log(std::string("ui: zmp:// links ") + (ok ? "now open this game" : "could not be registered"));
    return ok;
#else
    return false;
#endif
}

#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
namespace {

// The zmp: link soh.exe was opened with ("" none), in UTF-8.
std::string LinkFromCommandLine() {
    int argc = 0;
    LPWSTR* argv = CommandLineToArgvW(GetCommandLineW(), &argc);
    if (argv == nullptr) {
        return "";
    }
    std::string link;
    for (int i = 1; i < argc; i++) {
        std::wstring w = argv[i];
        if (w.size() > 4 && w.size() < 2048 && _wcsnicmp(w.c_str(), L"zmp:", 4) == 0) {
            int len = WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(), nullptr, 0, nullptr, nullptr);
            link.assign((size_t)len, '\0');
            WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(), link.data(), len, nullptr, nullptr);
            break;
        }
    }
    LocalFree(argv);
    return link;
}

// One running copy per game folder (the working directory is the game's data folder): its pipe and mutex names carry
// a digest of that folder, so test instances in their own folders, or two copies of the game installed side by side,
// never hand links to each other.
std::wstring InstanceName(const wchar_t* prefix) {
    wchar_t dir[4096];
    DWORD n = GetCurrentDirectoryW(4096, dir);
    wchar_t full[4096];
    DWORD m = n > 0 && n < 4096 ? GetFullPathNameW(dir, 4096, full, nullptr) : 0;
    std::wstring path = m > 0 && m < 4096 ? std::wstring(full, m) : std::wstring(L".");
    while (path.size() > 3 && (path.back() == L'\\' || path.back() == L'/')) {
        path.pop_back();
    }
    CharLowerBuffW(path.data(), (DWORD)path.size());
    int len = WideCharToMultiByte(CP_UTF8, 0, path.c_str(), (int)path.size(), nullptr, 0, nullptr, nullptr);
    std::string utf8((size_t)len, '\0');
    WideCharToMultiByte(CP_UTF8, 0, path.c_str(), (int)path.size(), utf8.data(), len, nullptr, nullptr);
    auto d = Crypto::Sha256((const uint8_t*)utf8.data(), utf8.size());
    std::string hex = Crypto::Hex(d.data(), 8);
    return std::wstring(prefix) + std::wstring(hex.begin(), hex.end());
}

std::wstring PipeName() {
    return InstanceName(L"\\\\.\\pipe\\zmp-link-");
}

std::wstring MutexName() {
    return InstanceName(L"Local\\zmp-instance-");
}

std::mutex sLinkMutex;
std::vector<std::string> sLinksIn; // links handed over by a second copy, for the game thread (OnFrame)
HANDLE sInstanceMutex = nullptr;

void LinkServerLoop(std::wstring name) {
    for (;;) {
        HANDLE p = CreateNamedPipeW(name.c_str(), PIPE_ACCESS_DUPLEX | FILE_FLAG_FIRST_PIPE_INSTANCE,
                                    PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS,
                                    1, 4096, 4096, 0, nullptr);
        if (p == INVALID_HANDLE_VALUE) {
            Log("ui: zmp:// links of a second copy cannot be received (pipe error " + std::to_string(GetLastError()) +
                "): a link opens another copy of the game");
            return;
        }
        bool connected = ConnectNamedPipe(p, nullptr) ? true : GetLastError() == ERROR_PIPE_CONNECTED;
        if (connected) {
            char buf[2048];
            DWORD n = 0;
            if (ReadFile(p, buf, sizeof(buf), &n, nullptr) && n > 4 && _strnicmp(buf, "zmp:", 4) == 0) {
                {
                    std::lock_guard<std::mutex> lk(sLinkMutex);
                    sLinksIn.emplace_back(buf, n);
                }
                DWORD w = 0;
                WriteFile(p, "ok", 2, &w, nullptr);
                FlushFileBuffers(p);
            }
            DisconnectNamedPipe(p);
        }
        CloseHandle(p);
    }
}

// Joins with a link and shows the room window (the player sees the room, or why the code did not join).
bool JoinLink(const std::string& link, const char* how) {
    snprintf(sCode, sizeof(sCode), "%s", link.c_str());
    bool ok = JoinWithCode(link, "", &sCodeMessage);
    Log(std::string("ui: ") + how + ": " + (ok ? "joining" : "not joined (" + sCodeMessage + ")"));
    SelectTab("join");
    auto win = Ship::Context::GetRawInstance()->GetWindow()->GetGui()->GetGuiWindow("Sala ZMP");
    if (win != nullptr) {
        win->Show();
    }
    return ok;
}

// The game's window comes to the front for a link the player clicked (never in test instances: they never take the
// focus, D-063). The second copy allowed it (AllowSetForegroundWindow) before ending.
void BringToFront() {
    char value[8] = {};
    if (GetEnvironmentVariableA("ZMP_NO_ACTIVATE", value, sizeof(value)) != 0 && value[0] == '1') {
        return;
    }
    EnumWindows(
        [](HWND hwnd, LPARAM) -> BOOL {
            DWORD pid = 0;
            GetWindowThreadProcessId(hwnd, &pid);
            if (pid == GetCurrentProcessId() && IsWindowVisible(hwnd) && GetWindow(hwnd, GW_OWNER) == nullptr) {
                if (IsIconic(hwnd)) {
                    ShowWindow(hwnd, SW_RESTORE);
                }
                SetForegroundWindow(hwnd);
                return FALSE;
            }
            return TRUE;
        },
        0);
}

} // namespace
#endif

void StartLinkServer() {
#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
    if (sInstanceMutex != nullptr) {
        return;
    }
    sInstanceMutex = CreateMutexW(nullptr, FALSE, MutexName().c_str()); // (held until the process ends)
    if (sInstanceMutex != nullptr && GetLastError() == ERROR_ALREADY_EXISTS) {
        Log("ui: another copy of the game runs in this folder: zmp:// links go to it");
        return;
    }
    std::thread(LinkServerLoop, PipeName()).detach(); // (blocked on the pipe; ends with the process)
#endif
}

bool JoinFromCommandLine() {
#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
    std::string link = LinkFromCommandLine();
    if (link.empty()) {
        return false;
    }
    return JoinLink(link, "opened with a zmp:// link");
#else
    return false;
#endif
}

#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
static void TakeHandedLinks() {
    std::vector<std::string> links;
    {
        std::lock_guard<std::mutex> lk(sLinkMutex);
        links.swap(sLinksIn);
    }
    for (auto& l : links) {
        JoinLink(l, "a zmp:// link opened while the game runs (handed over by a second copy)");
        BringToFront();
    }
}
#endif

} // namespace Zmp::Ui

// Before anything else in main (src/code/main.c): when soh.exe is opened with a zmp:// link and the game already runs
// in its folder, the link goes to that copy through its pipe and this process ends before it opens any window. A
// link opened by the browser may come with the browser's working directory: the game's folder is then the
// executable's (where its shipofharkinian.json is). Returns 1 when the link was handed over.
extern "C" int Zmp_LinkHandOff(void) {
#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
    using namespace Zmp::Ui;
    std::string link = LinkFromCommandLine();
    if (link.empty()) {
        return 0;
    }
    if (GetFileAttributesW(L"shipofharkinian.json") == INVALID_FILE_ATTRIBUTES) {
        wchar_t exe[4096];
        DWORD n = GetModuleFileNameW(nullptr, exe, 4096);
        std::wstring dir(exe, n > 0 && n < 4096 ? n : 0);
        size_t slash = dir.find_last_of(L"\\/");
        if (slash != std::wstring::npos) {
            dir.resize(slash);
            SetCurrentDirectoryW(dir.c_str());
        }
    }
    Zmp_InstallBootLog();
    HANDLE m = OpenMutexW(SYNCHRONIZE, FALSE, MutexName().c_str());
    if (m == nullptr) {
        Zmp_BootMark("boot: zmp:// link, no other copy in this folder: this one joins");
        return 0;
    }
    CloseHandle(m);
    std::wstring name = PipeName();
    HANDLE p = INVALID_HANDLE_VALUE;
    ULONGLONG until = GetTickCount64() + 8000; // (the other copy may still be starting: its pipe comes with Zmp_Init)
    while (GetTickCount64() < until) {
        p = CreateFileW(name.c_str(), GENERIC_READ | GENERIC_WRITE, 0, nullptr, OPEN_EXISTING, 0, nullptr);
        if (p != INVALID_HANDLE_VALUE) {
            break;
        }
        if (GetLastError() == ERROR_PIPE_BUSY) {
            WaitNamedPipeW(name.c_str(), 1000);
        } else {
            Sleep(100);
        }
    }
    if (p == INVALID_HANDLE_VALUE) {
        Zmp_BootMark("boot: zmp:// link, another copy runs here but its pipe did not answer in 8 s: this one opens");
        return 0;
    }
    DWORD mode = PIPE_READMODE_MESSAGE;
    SetNamedPipeHandleState(p, &mode, nullptr, nullptr);
    DWORD w = 0;
    char reply[8] = {};
    DWORD r = 0;
    bool ok = WriteFile(p, link.data(), (DWORD)link.size(), &w, nullptr) && w == link.size() &&
              ReadFile(p, reply, sizeof(reply), &r, nullptr) && r == 2 && memcmp(reply, "ok", 2) == 0;
    ULONG pid = 0;
    if (ok && GetNamedPipeServerProcessId(p, &pid)) {
        AllowSetForegroundWindow(pid); // (this process was started by the player's click: it may pass the focus on)
    }
    CloseHandle(p);
    Zmp_BootMark(ok ? "boot: zmp:// link handed over to the copy already running in this folder: this one ends"
                    : "boot: zmp:// link, the running copy did not take it: this one opens");
    return ok ? 1 : 0;
#else
    return 0;
#endif
}

namespace Zmp::Ui {

void OnFrame() {
    NetStatus st = Client::Get().GetStatus();
    if (st.renameSeq != sSeenRenameSeq) {
        sSeenRenameSeq = st.renameSeq;
        if (st.renameResult == "ok") {
            CVarSetString(ZMP_CVAR_NAME, st.name.c_str());
            SaveSoon();
            sProfileMessage = T("profile.renamed");
            Chat::Post(Chat::Category::System, std::string(T("profile.renamed")) + ": " + st.name);
        } else {
            std::map<std::string, std::string> args = { { "name", sNewName } };
            sProfileMessage = Text::Rejection(st.renameResult, args, st.renameResult);
        }
    }
    if ((int)st.state != sSeenState) {
        sSeenState = (int)st.state;
        if (st.state == NetState::Connected) {
            // (the form that worked: a bare domain that fell back to wss is remembered as wss)
            std::string addr = st.via.empty() ? st.host : st.via;
            bool fromHost = false;
#ifndef __EMSCRIPTEN__
            fromHost = Host::Get().running && addr.rfind("127.0.0.1:", 0) == 0;
#endif
            if (!ServerLocked() && !fromHost) {
                CVarSetString(ZMP_CVAR_SERVER_HOST, addr.c_str()); // (an invite code's address that answered)
                RecentRemember(addr, st.room);
            }
        }
    }
#ifndef __EMSCRIPTEN__
    Host::Poll();
#endif
#if defined(_WIN32) && !defined(__EMSCRIPTEN__)
    TakeHandedLinks();
#endif
}

void DrawMultiplayer() {
    if (ImGui::BeginTabBar("##ZmpTabs")) {
        auto tab = [](const char* key, const char* id) {
            ImGuiTabItemFlags f = sTabRequest == id ? ImGuiTabItemFlags_SetSelected : 0;
            bool open = ImGui::BeginTabItem((std::string(T(key)) + "###" + id).c_str(), nullptr, f);
            return open;
        };
        if (tab("tab.join", "join")) {
            DrawJoin();
            ImGui::EndTabItem();
        }
        if (tab("tab.host", "host")) {
            DrawHost();
            ImGui::EndTabItem();
        }
        if (tab("tab.profile", "profile")) {
            DrawProfile();
            ImGui::EndTabItem();
        }
        if (tab("tab.advanced", "advanced")) {
            DrawAdvanced();
            ImGui::EndTabItem();
        }
        sTabRequest.clear();
        ImGui::EndTabBar();
    }
}

} // namespace Zmp::Ui
