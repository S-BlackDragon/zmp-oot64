"""Builds the Windows player package dist/ZMP-<version>-windows.zip from the cpack zip of the distribution build.

    python tools/package_windows.py --build <cpack zip> --version 0.6.2

Input: the zip that `cpack -G ZIP -C Release` writes for a build configured with -DZMP_HARNESS=OFF
(see docs-zmp/en/build.md). Output: soh.exe, soh.o2r (Ship of Harkinian's own data), the ROM converter's descriptions
(assets/), gamecontrollerdb.txt, the upstream readme, PLAYING.txt / JUGAR.txt, the license files and a first-run
shipofharkinian.json that opens the ZMP room window with an EMPTY server address (each group types its own host).

Never includes a ROM, an oot.o2r or the debug symbols; tools/check_release.py runs on the result and the zip is
deleted if it finds anything.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
HERE = Path(__file__).resolve().parent
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


def first_run_config() -> dict:
    return {
        "ConfigVersion": 7,
        "CVars": {
            "gOpenWindows": {"ZmpRoom": 1},
            "gZmp": {"Room": "zmp", "Server": {"Host": "", "Port": 47100}, "Overlay": 1, "AutoConnect": 0},
        },
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--build", type=Path, required=True, help="cpack zip of the distribution build")
    ap.add_argument("--version", required=True)
    ap.add_argument("--out", type=Path, default=ROOT / "dist")
    args = ap.parse_args()

    name = f"ZMP-{args.version}-windows"
    stage = args.out / name
    if stage.exists():
        shutil.rmtree(stage)
    stage.mkdir(parents=True)
    with zipfile.ZipFile(args.build) as z:
        for info in z.infolist():
            n = info.filename
            if n.startswith("debug/") or n.endswith(".pdb") or (n.endswith(".o2r") and n != "soh.o2r"):
                continue
            if n.lower().endswith((".z64", ".n64", ".v64")):
                raise SystemExit(f"refusing to package a ROM: {n}")
            z.extract(info, stage)
    if not (stage / "soh.exe").exists() or not (stage / "soh.o2r").exists():
        raise SystemExit("the cpack zip has no soh.exe / soh.o2r at its root: is it the right zip?")
    if (stage / "readme.txt").exists():
        (stage / "readme.txt").rename(stage / "readme-ship-of-harkinian.txt")
    shutil.copy2(HERE / "PLAYING.txt", stage / "PLAYING.txt")
    shutil.copy2(HERE / "JUGAR.txt", stage / "JUGAR.txt")
    for dest, src in LICENSE_FILES.items():
        (stage / dest).parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / src, stage / dest)
    (stage / "shipofharkinian.json").write_text(json.dumps(first_run_config(), indent=4), encoding="utf-8")

    zpath = args.out / f"{name}.zip"
    if zpath.exists():
        zpath.unlink()
    with zipfile.ZipFile(zpath, "w", zipfile.ZIP_DEFLATED, compresslevel=6) as z:
        for p in sorted(stage.rglob("*")):
            if p.is_file():
                z.write(p, Path(name) / p.relative_to(stage))
    check = subprocess.run([sys.executable, str(HERE / "check_release.py"), str(zpath)])
    if check.returncode != 0:
        zpath.unlink()
        raise SystemExit("check_release found forbidden content; the zip was deleted")
    sha = hashlib.sha256(zpath.read_bytes()).hexdigest()
    exe = hashlib.sha256((stage / "soh.exe").read_bytes()).hexdigest()
    (args.out / f"{name}.sha256.txt").write_text(f"{sha}  {zpath.name}\n{exe}  soh.exe\n", encoding="utf-8")
    shutil.rmtree(stage)
    print(f"{zpath} ({zpath.stat().st_size / 1e6:.1f} MB) sha256 {sha}")
    print(f"soh.exe sha256 {exe} (every player of a room needs this same soh.exe)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
