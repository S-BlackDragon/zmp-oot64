// ZMP web (D-238, D-241): what the page hands the game.
//
// Module.zmp (filled by the page's connection form, shell.html) and the site's configuration below (zmp-config.json
// and the URL, Module.zmpConfig). The ROM is never named here: the player picks it from their own computer
// (rom-setup.js), it is converted in memory and never stored.
// Module.zmp.buildHash is filled here with the SHA-256 of soh.wasm (first 16 hex digits): the identity of this build
// in the HELLO handshake, as the executable's hash is for the native game (same wasm = same hash).

(function () {
    const params = new URLSearchParams(location.search);
    const cfg = Object.assign({}, Module.zmp || {});
    // (o2r with harness=1: an already extracted oot.o2r by URL and the test harness, for test builds only)
    for (const key of ["o2r", "harness", "mutant"]) {
        if (params.has(key)) {
            cfg[key] = params.get(key);
        }
    }
    Module.zmp = cfg;
})();

// The site's configuration (docs/PRODUCTO_PUBLICO.md 6, D-262; guide docs-zmp/*/10-*.md): an optional zmp-config.json
// next to the page, then the URL parameters it allows, then the URL fragment (#invite=, #pw=: they never reach any
// server and are wiped from the address bar). Every value is checked: a bad one is ignored, said in the console and
// in a notice on the page, and never stops the game. With no file the game presets nothing (it asks for a server).
// What comes out goes to Module.zmp for the game (Zmp.cpp ApplyWebConfig): host, room, name, lockServer, lockName,
// roomAsk, passwordPolicy, password, invite, lang, autojoin, updateCheck, updateSource.
Module.zmpConfig = (() => {
    const problems = [];
    const bad = (where, why) => {
        problems.push(where + ": " + why);
        console.warn("ZMP config: " + where + ": " + why + " (ignored)");
    };
    const isBool = (v) => typeof v === "boolean";
    const isStr = (v, max) => typeof v === "string" && v.length <= max;
    // An address of the shared grammar (Net/Address.h) as a site would give it: ws:// or wss:// URL, or host[:port].
    const goodAddress = (v) =>
        typeof v === "string" && v.length > 0 && v.length <= 255 && !/\s/.test(v) &&
        (/^wss?:\/\/[^/?#\s]+/i.test(v) || /^(\[[0-9a-f:.]+\]|[a-z0-9._-]+)(:\d{1,5})?$/i.test(v));
    const known = ["version", "servers", "lockServer", "room", "password", "name", "assets", "ui", "url", "embed",
                   "updates", "$schema"];

    function fromFile(file, out) {
        if (file === null || typeof file !== "object" || Array.isArray(file)) {
            bad("zmp-config.json", "is not a JSON object");
            return;
        }
        for (const k of Object.keys(file)) {
            if (!known.includes(k)) {
                bad(k, "unknown key");
            }
        }
        if ("version" in file && file.version !== 1) {
            bad("version", "must be 1");
        }
        if ("servers" in file) {
            if (!Array.isArray(file.servers)) {
                bad("servers", "must be a list");
            } else {
                const good = [];
                file.servers.forEach((srv, i) => {
                    if (!srv || !goodAddress(srv.address)) {
                        bad("servers[" + i + "].address", "not a server address (wss://..., ws://..., host or host:port)");
                    } else {
                        good.push(srv);
                    }
                });
                const pick = good.find((srv) => srv.default === true) || good[0];
                if (pick) {
                    out.host = pick.address;
                }
            }
        }
        if ("lockServer" in file) {
            isBool(file.lockServer) ? (out.lockServer = file.lockServer) : bad("lockServer", "must be true or false");
        }
        if ("room" in file) {
            const r = file.room;
            if (r === "ask") {
                out.roomAsk = true;
            } else if (typeof r === "string") {
                r.length >= 1 && r.length <= 64 ? (out.room = r) : bad("room", "1 to 64 characters, or \"ask\"");
            } else if (r && typeof r === "object") {
                if ("default" in r) {
                    isStr(r.default, 64) ? (out.room = r.default) : bad("room.default", "0 to 64 characters");
                }
                if ("ask" in r) {
                    isBool(r.ask) ? (out.roomAsk = r.ask) : bad("room.ask", "must be true or false");
                }
                if ("suggestRandom" in r && r.suggestRandom === true && !out.room) {
                    out.room = "sala-" + Math.random().toString(36).slice(2, 6);
                }
            } else {
                bad("room", "a name, \"ask\" or {default, ask}");
            }
        }
        if ("password" in file) {
            const p = file.password;
            const policies = { none: "none", ask: "optional", optional: "optional", required: "required", given: "given" };
            if (!p || typeof p !== "object" || !(p.policy in policies)) {
                bad("password.policy", "one of none, ask, required, given");
            } else {
                out.passwordPolicy = policies[p.policy];
                if ("value" in p) {
                    isStr(p.value, 64) ? (out.password = p.value) : bad("password.value", "at most 64 characters");
                }
                if ("fromUrlFragment" in p) {
                    isBool(p.fromUrlFragment) ? (out.pwFromFragment = p.fromUrlFragment)
                                              : bad("password.fromUrlFragment", "must be true or false");
                }
            }
        }
        if ("name" in file) {
            const n = file.name;
            if (!n || typeof n !== "object") {
                bad("name", "must be {default, lock}");
            } else {
                if ("default" in n) {
                    isStr(n.default, 32) ? (out.name = n.default) : bad("name.default", "at most 32 characters");
                }
                if ("lock" in n) {
                    isBool(n.lock) ? (out.lockName = n.lock) : bad("name.lock", "must be true or false");
                }
            }
        }
        if ("ui" in file) {
            const u = file.ui || {};
            if ("language" in u) {
                ["auto", "es", "en"].includes(u.language) ? (out.lang = u.language) : bad("ui.language", "auto, es or en");
            }
            if ("title" in u) {
                isStr(u.title, 80) ? (out.title = u.title) : bad("ui.title", "at most 80 characters");
            }
        }
        if ("url" in file) {
            const allowed = ["server", "room", "name", "lang", "autojoin", "invite", "pw"];
            if (!file.url || !Array.isArray(file.url.allow) || !file.url.allow.every((x) => allowed.includes(x))) {
                bad("url.allow", "a list of: " + allowed.join(", "));
            } else {
                out.urlAllow = file.url.allow;
            }
        }
        if ("updates" in file) {
            const up = file.updates || {};
            if ("check" in up) {
                isBool(up.check) ? (out.updateCheck = up.check) : bad("updates.check", "must be true or false");
            }
            if ("source" in up) {
                typeof up.source === "string" && /^https:\/\/\S+$/.test(up.source) && up.source.length <= 255
                    ? (out.updateSource = up.source)
                    : bad("updates.source", "an https:// address");
            }
        }
        if ("assets" in file) {
            const as = file.assets || {};
            if ("remember" in as && !["ask", "never"].includes(as.remember)) {
                bad("assets.remember", "ask or never");
            }
        }
        if ("embed" in file) {
            const e = file.embed || {};
            if ("parentOrigins" in e && !Array.isArray(e.parentOrigins)) {
                bad("embed.parentOrigins", "must be a list");
            }
        }
    }

    function fromUrl(out) {
        const params = new URLSearchParams(location.search);
        const allow = out.urlAllow || ["server", "room", "name", "lang", "autojoin", "invite", "pw"];
        const take = (key, check, set, why) => {
            if (!params.has(key)) {
                return;
            }
            const v = params.get(key);
            if (!allow.includes(key)) {
                bad("?" + key, "not allowed by this site");
            } else if (!check(v)) {
                bad("?" + key, why);
            } else {
                set(v);
            }
        };
        // (host/port: the older parameters, still read: a ws/wss URL or host plus port)
        if (params.has("host") && allow.includes("server")) {
            const h = params.get("host"), port = params.get("port");
            const addr = port && !/^wss?:/i.test(h) ? (h.includes(":") && !h.startsWith("[") ? "[" + h + "]" : h) + ":" + port : h;
            goodAddress(addr) ? (out.host = addr) : bad("?host", "not a server address");
            if (out.lockServer && out.host !== addr) {
                // (never happens: lockServer wins below)
            }
        }
        const lockedHost = out.lockServer ? out.host : undefined;
        take("server", goodAddress, (v) => (out.host = v), "not a server address (wss://..., ws://..., host:port)");
        if (out.lockServer && lockedHost !== undefined && out.host !== lockedHost) {
            bad("?server", "this site fixes its server");
            out.host = lockedHost;
        }
        take("room", (v) => v.length >= 1 && v.length <= 64, (v) => {
            out.room = v;
            out.roomFromUrl = true;
        }, "1 to 64 characters");
        take("name", (v) => v.length >= 1 && v.length <= 32, (v) => (out.lockName ? bad("?name", "this site fixes the name") : (out.name = v)), "1 to 32 characters");
        take("lang", (v) => ["auto", "es", "en"].includes(v), (v) => (out.lang = v), "auto, es or en");
        take("autojoin", (v) => v === "0" || v === "1", (v) => (out.autojoin = v === "1"), "0 or 1");
        // the fragment: never sent to any server; wiped from the address bar once read
        const frag = new URLSearchParams(location.hash.replace(/^#/, ""));
        let wipe = false;
        if (frag.has("invite")) {
            wipe = true;
            allow.includes("invite") ? (out.invite = frag.get("invite").slice(0, 512)) : bad("#invite", "not allowed by this site");
        }
        if (frag.has("pw")) {
            wipe = true;
            const v = frag.get("pw");
            if (!allow.includes("pw") || out.pwFromFragment === false) {
                bad("#pw", "not allowed by this site");
            } else if (v.length > 64) {
                bad("#pw", "at most 64 characters");
            } else {
                out.password = v;
            }
        }
        if (wipe && history.replaceState) {
            history.replaceState(null, "", location.pathname + location.search);
        }
    }

    function show() {
        if (problems.length === 0) {
            return;
        }
        const box = document.createElement("div");
        box.id = "zmp-config-notice";
        box.setAttribute("role", "status");
        box.style.cssText = "position:fixed;top:8px;left:50%;transform:translateX(-50%);z-index:50;max-width:min(640px,92vw);" +
            "padding:10px 14px;border-radius:8px;background:#3a1f1f;color:#ffd7d7;font:13px system-ui,sans-serif;" +
            "box-shadow:0 6px 24px rgba(0,0,0,.5)";
        const es = (navigator.language || "").startsWith("es");
        box.textContent = (es ? "Configuración del sitio: se ignoró " : "Site configuration: ignored ") + problems.join("; ");
        const close = () => box.remove();
        box.addEventListener("click", close);
        setTimeout(close, 20000);
        (document.body || document.documentElement).appendChild(box);
    }

    async function load() {
        const out = {};
        try {
            const r = await fetch(new URL("zmp-config.json", location.href), { cache: "no-cache" });
            if (r.ok) {
                const text = await r.text();
                let file;
                try {
                    file = JSON.parse(text);
                } catch (e) {
                    bad("zmp-config.json", "is not valid JSON (" + e.message + ")");
                }
                if (file !== undefined) {
                    fromFile(file, out);
                }
            } else if (r.status !== 404) {
                bad("zmp-config.json", "HTTP " + r.status);
            }
        } catch (e) {
            // (no file and no server answer: the defaults)
        }
        fromUrl(out);
        if (out.passwordPolicy === "given" && !out.password) {
            out.passwordPolicy = "required"; // (nothing was given: the player types it)
        }
        const z = Module.zmp;
        // (a host page that sets Module.zmp.host and .room itself, as before: it joins straight away)
        const pageJoins = !!(z.host && z.room);
        for (const [k, v] of Object.entries(out)) {
            if (k !== "urlAllow" && k !== "pwFromFragment" && k !== "roomFromUrl") {
                z[k] = typeof v === "boolean" ? (v ? "1" : "0") : String(v);
            }
        }
        // Joining without asking: the site's server (fixed) and a room, or ?autojoin=1, and no password left to ask.
        const pwOk = out.passwordPolicy !== "required" && out.passwordPolicy !== "given" || !!out.password;
        const auto = out.autojoin !== undefined ? out.autojoin
                                                : (out.lockServer === true || out.roomFromUrl === true || pageJoins);
        z.autojoin = (out.invite || (auto && z.host && z.room && !out.roomAsk && pwOk)) ? "1" : "0";
        if (out.title) {
            document.title = out.title;
        }
        Module.zmpConfig.problems = problems.slice();
        Module.zmpConfig.applied = Object.assign({}, out, { password: out.password ? "(set)" : undefined });
        console.log("ZMP config:", JSON.stringify(Module.zmpConfig.applied));
        if (document.readyState === "loading") {
            document.addEventListener("DOMContentLoaded", show);
        } else {
            show();
        }
    }
    return { load, problems, applied: {} };
})();

Module["preRun"] = [].concat(Module["preRun"] || [], () => {
    addRunDependency("zmp-config");
    Module.zmpConfig.load().finally(() => removeRunDependency("zmp-config"));
});

// Test builds (D-242): commands to the game's harness and their answers, through two exported functions.
// zmpHarness.call({cmd, id, ...}) resolves with the answer that carries the same id.
Module.zmpHarness = (() => {
    const waiting = new Map();
    let timer = null;
    const poll = () => {
        let replies;
        try {
            replies = JSON.parse(Module.ccall("Zmp_HarnessWebPoll", "string", [], []));
        } catch (e) {
            return;
        }
        for (const r of replies) {
            const w = waiting.get(r.id);
            if (w) {
                waiting.delete(r.id);
                w(r);
            }
        }
        if (waiting.size === 0 && timer !== null) {
            clearInterval(timer);
            timer = null;
        }
    };
    return {
        available: () => typeof Module._Zmp_HarnessWebRequest === "function",
        call(req) {
            return new Promise((resolve, reject) => {
                if (!this.available()) {
                    reject(new Error("this build has no harness"));
                    return;
                }
                waiting.set(req.id, resolve);
                Module.ccall("Zmp_HarnessWebRequest", null, ["string"], [JSON.stringify(req)]);
                if (timer === null) {
                    timer = setInterval(poll, 5);
                }
            });
        },
    };
})();

Module["preRun"] = [].concat(Module["preRun"] || [], () => {
    addRunDependency("zmp-build-hash");
    const url = typeof Module.locateFile === "function" ? Module.locateFile("soh.wasm", "") : "soh.wasm";
    fetch(url)
        .then((r) => (r.ok ? r.arrayBuffer() : Promise.reject(new Error("HTTP " + r.status))))
        .then((bytes) => crypto.subtle.digest("SHA-256", bytes))
        .then((digest) => {
            Module.zmp.buildHash = [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("")
                .slice(0, 16);
        })
        .catch((e) => {
            console.warn("ZMP: couldn't hash soh.wasm; the build identity will be 'missing'.", e);
        })
        .finally(() => removeRunDependency("zmp-build-hash"));
});
