#include "ZmpText.h"

#include <cstring>
#include <unordered_map>
#include <vector>

#include <libultraship/bridge/consolevariablebridge.h>

#include "soh/Zmp/ZmpCVars.h"

#if defined(__EMSCRIPTEN__)
#include <emscripten.h>
#elif defined(_WIN32)
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#endif

namespace Zmp::Text {

namespace {

struct Pair {
    const char* es;
    const char* en;
};

// (ASCII in the Spanish texts: the game's ImGui font has no accented letters)
const std::unordered_map<std::string, Pair>& Table() {
    static const std::unordered_map<std::string, Pair> t = {
        // tabs and sections
        { "tab.join", { "Unirse", "Join" } },
        { "tab.host", { "Alojar", "Host" } },
        { "tab.profile", { "Perfil", "Profile" } },
        { "tab.advanced", { "Avanzado", "Advanced" } },
        { "join.code", { "Unirse con codigo", "Join with a code" } },
        { "join.code.hint",
          { "Pega aqui el codigo de invitacion (ZMP-...) o el enlace zmp://",
            "Paste the invite code (ZMP-...) or the zmp:// link here" } },
        { "join.code.button", { "Unirse con este codigo", "Join with this code" } },
        { "join.code.ok", { "Codigo valido: sala %s en %s", "Valid code: room %s at %s" } },
        { "join.code.hintpw",
          { "El anfitrion dejo una pista de la contrasena: %s", "The host left a hint for the password: %s" } },
        { "join.lan", { "En tu red local", "On your local network" } },
        { "join.lan.none", { "Buscando partidas en esta red...", "Looking for games on this network..." } },
        { "join.lan.enter", { "Entrar", "Join" } },
        { "join.address", { "Por direccion", "By address" } },
        { "join.server", { "Servidor", "Server" } },
        { "join.server.help",
          { "vale: host, host:puerto, IP, [IPv6]:puerto, ws://, wss://",
            "accepted: host, host:port, IP, [IPv6]:port, ws://, wss://" } },
        { "join.server.locked", { "(fijado por este sitio)", "(set by this site)" } },
        { "join.room", { "Sala", "Room" } },
        { "join.password", { "Contrasena (opcional)", "Password (optional)" } },
        { "join.password.required", { "Contrasena", "Password" } },
        { "join.password.needed",
          { "Este sitio pide una contrasena para conectar", "This site asks for a password to connect" } },
        { "join.recent", { "Recientes", "Recent" } },
        { "join.recent.none", { "(ninguno todavia)", "(none yet)" } },
        { "join.connect", { "Conectar", "Connect" } },
        { "join.disconnect", { "Desconectar", "Disconnect" } },
        { "join.badaddress", { "Esa direccion no es valida", "That address is not valid" } },
        { "host.title", { "Alojar partida", "Host a game" } },
        { "host.web",
          { "El navegador no puede alojar: pide a quien aloje (con la app de escritorio) su codigo de "
            "invitacion, o usa el servidor de tu sitio.",
            "A browser cannot host: ask whoever hosts (with the desktop app) for their invite code, or use "
            "your site's server." } },
        { "host.scope", { "Quien puede entrar", "Who can join" } },
        { "host.scope.lan",
          { "Mi red local y quien tenga el codigo (se intenta abrir el puerto en el router)",
            "My local network and whoever has the code (the router is asked to open the port)" } },
        { "host.scope.lanshort",
          { "Mi red local y quien tenga el codigo", "My local network and whoever has the code" } },
        { "host.scope.lanhelp",
          { "Se pide al router que abra el puerto; si no se puede, la pantalla lo dice y explica que hacer.",
            "The router is asked to open the port; if it cannot be done, this screen says so and what to do." } },
        { "host.scope.local", { "Solo este PC (pruebas)", "This PC only (tests)" } },
        { "host.room", { "Sala", "Room" } },
        { "host.port", { "Puerto", "Port" } },
        { "host.start", { "Alojar y jugar", "Host and play" } },
        { "host.stop", { "Dejar de alojar", "Stop hosting" } },
        { "host.starting", { "Arrancando el servidor...", "Starting the server..." } },
        { "host.running", { "Alojando la sala %s en el puerto %d", "Hosting room %s on port %d" } },
        { "host.code", { "Codigo de invitacion (pasaselo a tus amigos)", "Invite code (give it to your friends)" } },
        { "host.copy", { "Copiar codigo", "Copy code" } },
        { "host.copylink", { "Copiar enlace", "Copy link" } },
        { "host.copied", { "Copiado", "Copied" } },
        { "host.reach", { "Desde internet", "From the internet" } },
        { "host.password", { "Contrasena de la sala", "Room password" } },
        { "host.nopassword", { "(sin contrasena)", "(no password)" } },
        { "host.missing",
          { "No se encuentra zmp-server.exe junto al juego: vuelve a descargar la app.",
            "zmp-server.exe is not next to the game: download the app again." } },
        { "host.failed", { "El servidor no arranco: %s", "The server did not start: %s" } },
        { "host.exited", { "El servidor se ha cerrado (codigo %d)", "The server closed (code %d)" } },
        { "host.lanbrowser", { "Desde un navegador de tu red: %s", "From a browser on your network: %s" } },
        { "profile.name", { "Nombre", "Name" } },
        { "profile.name.help",
          { "Se ve sobre tu Link; se puede cambiar en partida y lo ven todos al instante",
            "Shown over your Link; you can change it while playing and everyone sees it at once" } },
        { "profile.rename", { "Cambiar nombre", "Change name" } },
        { "profile.renamed", { "Nombre cambiado", "Name changed" } },
        { "profile.language", { "Idioma", "Language" } },
        { "status.connected", { "Conectado a %s (sala %s, %d jugadores)", "Connected to %s (room %s, %d players)" } },
        { "status.connecting", { "Conectando a %s...", "Connecting to %s..." } },
        { "status.disconnected", { "Desconectado", "Disconnected" } },
        // rejections (docs/PROTOCOLO_5.md 6) and this client's own errors
        { "rej.bad_hello",
          { "El servidor no entendio el saludo de este juego", "The server did not understand this game's greeting" } },
        { "rej.protocol_mismatch",
          { "Este juego y el servidor hablan versiones distintas del protocolo (servidor: %s a %s, sala: %s). "
            "Actualiza el juego o el servidor.",
            "This game and the server speak different protocol versions (server: %s to %s, room: %s). Update the game "
            "or the server." } },
        { "rej.bad_name",
          { "El nombre tiene que tener de 1 a %s caracteres", "The name must be 1 to %s characters long" } },
        { "rej.bad_room",
          { "La sala tiene que tener de 1 a %s caracteres", "The room must be 1 to %s characters long" } },
        { "rej.room_not_found", { "Ese servidor no tiene esa sala", "That server has no such room" } },
        { "rej.name_taken",
          { "Ya hay alguien llamado %s en la sala: cambia tu nombre",
            "Someone called %s is already in the room: change your name" } },
        { "rej.room_full", { "La sala esta llena (%s jugadores)", "The room is full (%s players)" } },
        { "rej.engine_mismatch",
          { "Esta sala es de otro motor (%s): escritorio con escritorio, navegador con navegador",
            "This room uses another engine (%s): desktop with desktop, browser with browser" } },
        { "rej.build_mismatch",
          { "Tu version del juego no es la de la sala (la sala usa %s, tu tienes %s): todos necesitais la misma "
            "version. Actualiza el juego.",
            "Your version of the game is not the room's (the room uses %s, you have %s): everyone needs the same "
            "version. Update the game." } },
        { "rej.oot_mismatch",
          { "Tu ROM no es la de la sala (la sala usa %s, tu %s): hace falta la misma version de la ROM",
            "Your ROM is not the room's (the room uses %s, you %s): the same ROM version is needed" } },
        { "rej.soh_mismatch",
          { "Tus archivos del juego (soh.o2r) no son los de la sala: reinstala la misma version",
            "Your game files (soh.o2r) are not the room's: reinstall the same version" } },
        { "rej.cvar_profile_mismatch",
          { "Tus opciones de juego bloqueadas no son las de la sala: usa la misma version",
            "Your locked game settings are not the room's: use the same version" } },
        { "rej.rando_seed_mismatch",
          { "La sala juega otra semilla del randomizer (%s)", "The room plays another randomizer seed (%s)" } },
        { "rej.password_required",
          { "Esta sala tiene contrasena: escribela y vuelve a conectar",
            "This room has a password: type it and connect again" } },
        { "rej.password_required_create",
          { "Este servidor solo deja crear salas con contrasena: escribe una",
            "This server only lets you create rooms with a password: type one" } },
        { "rej.bad_password", { "Contrasena incorrecta", "Wrong password" } },
        { "rej.server_password_required",
          { "Este servidor tiene contrasena propia: escribela en Avanzado",
            "This server has its own password: type it under Advanced" } },
        { "rej.bad_server_password", { "La contrasena del servidor no es correcta", "The server password is wrong" } },
        { "rej.too_many_attempts",
          { "Demasiados intentos fallidos: espera %s segundos", "Too many failed attempts: wait %s seconds" } },
        { "rej.rooms_limit",
          { "El servidor no admite mas salas (maximo %s)", "The server takes no more rooms (at most %s)" } },
        { "rej.too_many_connections",
          { "Demasiadas conexiones desde tu direccion (maximo %s)",
            "Too many connections from your address (at most %s)" } },
        { "rej.rooms_hidden",
          { "Este servidor no publica su lista de salas", "This server does not publish its room list" } },
        { "rej.auth_timeout",
          { "La entrada tardo demasiado: vuelve a conectar", "Joining took too long: connect again" } },
        { "rej.kicked", { "Te han sacado de la sala", "You were removed from the room" } },
        { "rej.bad_address",
          { "Esa direccion no es valida (vale: host, host:puerto, ws://, wss://)",
            "That address is not valid (accepted: host, host:port, ws://, wss://)" } },
        { "rej.unreachable", { "No se pudo conectar a %s", "Could not connect to %s" } },
        { "rej.tls_error",
          { "El certificado del servidor no es valido: no se conecta (wss)",
            "The server's certificate is not valid: not connecting (wss)" } },
        { "rej.not_zmp_server",
          { "Esa direccion responde, pero no es un servidor ZMP",
            "That address answers, but it is not a ZMP server" } },
        { "rej.unsupported", { "Este sistema no puede usar esa direccion", "This system cannot use that address" } },
        { "rej.connection_lost",
          { "Conexion perdida con el servidor: reintentando...", "Connection to the server lost: retrying..." } },
        { "rej.bad_confirm",
          { "El servidor no demostro conocer la contrasena de la sala: no es la sala que buscas",
            "The server could not prove it knows the room's password: it is not the room you want" } },
        { "inv.empty", { "Pega un codigo", "Paste a code" } },
        { "inv.character",
          { "El codigo tiene un caracter que no vale", "The code has a character that is not valid" } },
        { "inv.length", { "Al codigo le falta o le sobra algo", "The code is too short or too long" } },
        { "inv.checksum",
          { "El codigo no cuadra: revisa que este copiado entero y sin erratas",
            "The code does not check out: make sure it is copied whole and without typos" } },
        { "inv.version",
          { "El codigo es de una version mas nueva: actualiza el juego",
            "The code is from a newer version: update the game" } },
        { "inv.malformed", { "El codigo esta mal formado", "The code is malformed" } },
        { "inv.invalid", { "El codigo tiene un dato que no vale", "The code has an invalid field" } },
    };
    return t;
}

std::string Fill(const char* fmt, const std::vector<std::string>& args) {
    std::string out;
    size_t k = 0;
    for (const char* p = fmt; *p; p++) {
        if (p[0] == '%' && p[1] == 's') {
            out += k < args.size() && !args[k].empty() ? args[k] : "?";
            k++;
            p++;
        } else {
            out.push_back(*p);
        }
    }
    return out;
}

std::string Arg(const std::map<std::string, std::string>& args, const char* key) {
    auto it = args.find(key);
    return it == args.end() ? std::string() : it->second;
}

} // namespace

std::string Language() {
    std::string l = CVarGetString(ZMP_CVAR_LANGUAGE, "auto");
    if (l == "es" || l == "en") {
        return l;
    }
    static std::string sAuto = [] {
#if defined(__EMSCRIPTEN__)
        const char* nav = emscripten_run_script_string("String(navigator.language || 'en')");
        return std::string(nav != nullptr && strncmp(nav, "es", 2) == 0 ? "es" : "en");
#elif defined(_WIN32)
        return std::string(PRIMARYLANGID(GetUserDefaultUILanguage()) == LANG_SPANISH ? "es" : "en");
#else
        return std::string("en");
#endif
    }();
    return sAuto;
}

const char* T(const char* key) {
    auto& t = Table();
    auto it = t.find(key);
    if (it == t.end()) {
        return key;
    }
    return Language() == "es" ? it->second.es : it->second.en;
}

std::string Rejection(const std::string& reason, const std::map<std::string, std::string>& args,
                      const std::string& detail) {
    std::string key = "rej." + reason;
    if (reason == "password_required" && Arg(args, "create") == "1") {
        key = "rej.password_required_create";
    }
    if (Table().find(key) == Table().end()) {
        return detail.empty() ? reason : detail;
    }
    std::vector<std::string> a;
    if (reason == "protocol_mismatch") {
        a = { Arg(args, "min"), Arg(args, "max"), Arg(args, "room_proto") };
        if (args.empty()) {
            return Language() == "es"
                       ? "Este juego y el servidor hablan versiones distintas: actualiza el que sea mas viejo"
                       : "This game and the server speak different versions: update the older one";
        }
    } else if (reason == "bad_name" || reason == "bad_room") {
        a = { Arg(args, "max").empty() ? (reason == "bad_name" ? "32" : "64") : Arg(args, "max") };
    } else if (reason == "name_taken") {
        a = { Arg(args, "name") };
    } else if (reason == "room_full" || reason == "rooms_limit" || reason == "too_many_connections") {
        a = { Arg(args, "max").empty() ? "6" : Arg(args, "max") };
    } else if (reason == "engine_mismatch") {
        a = { Arg(args, "engine") };
    } else if (reason == "build_mismatch") {
        a = { Arg(args, "room_build"), Arg(args, "your_build") };
        if (args.empty() && !detail.empty()) {
            return std::string(Language() == "es" ? "Tu version del juego no es la de la sala: "
                                                  : "Your version of the "
                                                    "game is not the "
                                                    "room's: ") +
                   detail;
        }
    } else if (reason == "oot_mismatch") {
        a = { Arg(args, "room_rom"), Arg(args, "your_rom") };
        if (args.empty() && !detail.empty()) {
            return detail; // (a protocol 4 server: its Spanish text names both ROM versions)
        }
    } else if (reason == "rando_seed_mismatch") {
        a = { Arg(args, "room_seed") };
    } else if (reason == "too_many_attempts") {
        a = { Arg(args, "retry_after").empty() ? "60" : Arg(args, "retry_after") };
    } else if (reason == "unreachable") {
        a = { Arg(args, "address") };
    }
    return Fill(T(key.c_str()), a);
}

std::string InviteError(const std::string& error) {
    return T(("inv." + error).c_str());
}

} // namespace Zmp::Text
