// ZMP web (D-238, D-241): what the page hands the game.
//
// Module.zmp (filled by the page's connection form, shell.html) and/or URL parameters, the URL winning:
//   host, port, room, name   the zmp-server to connect to and the player's room and name; a room means AutoConnect.
//                            host may also be a full ws:// or wss:// URL (port is then ignored).
// The ROM is never named here: the player picks it from their own computer (rom-setup.js), it is converted in
// memory and never stored.
// Module.zmp.buildHash is filled here with the SHA-256 of soh.wasm (first 16 hex digits): the identity of this build
// in the HELLO handshake, as the executable's hash is for the native game (same wasm = same hash).

(function () {
    const params = new URLSearchParams(location.search);
    const cfg = Object.assign({}, Module.zmp || {});
    // (o2r with harness=1: an already extracted oot.o2r by URL and the test harness, for test builds only)
    for (const key of ["o2r", "host", "port", "room", "name", "harness", "mutant"]) {
        if (params.has(key)) {
            cfg[key] = params.get(key);
        }
    }
    Module.zmp = cfg;
})();

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
