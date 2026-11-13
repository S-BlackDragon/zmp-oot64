"""Assembles the browser version of ZMP into one folder that any static web server (or zmp-server -web) can serve.

    python web/make_web.py --build <web build dir>/soh --out dist/web

Takes the Emscripten build of the game (see docs-zmp/en/build.md) and writes:
  index.html            the game's page (built from soh/web/shell.html: connection form + local ROM picker)
  soh.js, soh.wasm      the game
  soh.data              Ship of Harkinian's own data (soh.o2r: its fonts, icons and menus; no Nintendo content)
  soh-extractor.js/.wasm, extractor-worker.js, yml/
                        the ROM converter that runs in the player's browser tab
  LICENSE-ZMP.txt, LICENSING.md, licenses/   the license notices
  manifest.json         sizes, SHA-256 of each file and the build hash every player of a room must share

Nothing of the original game goes in: each player picks their own ROM in the page, it is converted in memory and
never stored. The script refuses to write the folder if it finds a ROM, an extracted oot.o2r or anything that looks
like Nintendo data (tools/check_release.py runs on the result).
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
# License and notice files that travel with every game artifact (MIT requires the notices to be included).
LICENSE_FILES = {
    "LICENSE-ZMP.txt": "LICENSE-ZMP",
    "LICENSING.md": "LICENSING.md",
    "licenses/libultraship-LICENSE.txt": "libultraship/LICENSE",
    "licenses/fast3d-LICENSE.txt": "libultraship/src/fast/LICENSE.txt",
    "licenses/torch-LICENSE.txt": "torch/LICENSE",
    "licenses/stormlib-LICENSE.txt": "torch/lib/StormLib/LICENSE",
    "licenses/oot-randomizer-LICENSE.md": "soh/soh/Enhancements/randomizer/3drando/LICENSE.md",
}
FILES = ["soh.js", "soh.wasm", "soh.data", "soh-extractor.js", "soh-extractor.wasm", "extractor-worker.js"]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--build", required=True, help="the 'soh' folder of the Emscripten build (has soh.html)")
    ap.add_argument("--out", default=str(ROOT / "dist" / "web"))
    args = ap.parse_args()
    build, out = Path(args.build), Path(args.out)
    if not (build / "soh.html").exists():
        print(f"no web build at {build}: build it first (docs-zmp/en/build.md, 'Browser game')", file=sys.stderr)
        return 2
    if out.exists():
        shutil.rmtree(out)
    out.mkdir(parents=True)
    manifest: dict = {}
    page = (build / "soh.html").read_bytes()
    (out / "index.html").write_bytes(page)
    manifest["index.html"] = {"bytes": len(page), "sha256": hashlib.sha256(page).hexdigest()}
    for name in FILES:
        src = build / name
        if not src.exists():
            print(f"missing {src}", file=sys.stderr)
            return 3
        shutil.copyfile(src, out / name)
        data = src.read_bytes()
        manifest[name] = {"bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}
    if not (build / "yml").is_dir():
        print(f"missing {build / 'yml'}", file=sys.stderr)
        return 3
    shutil.copytree(build / "yml", out / "yml")
    yml = [p for p in (out / "yml").iterdir() if p.is_file()]
    manifest["yml/"] = {"bytes": sum(p.stat().st_size for p in yml), "files": len(yml)}
    for dest, src in LICENSE_FILES.items():
        (out / dest).parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / src, out / dest)
    # the identity of this build in the room handshake: same soh.wasm = same hash (players must all use the same)
    manifest["build_hash"] = manifest["soh.wasm"]["sha256"][:16]
    (out / "manifest.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    check = subprocess.run([sys.executable, str(ROOT / "tools" / "check_release.py"), str(out)])
    if check.returncode != 0:
        print("check_release found forbidden content: the folder must not be published", file=sys.stderr)
        return 4
    total = sum(v["bytes"] for k, v in manifest.items() if isinstance(v, dict))
    print(f"browser build written to {out}: {total / 1e6:.1f} MB, build hash {manifest['build_hash']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
