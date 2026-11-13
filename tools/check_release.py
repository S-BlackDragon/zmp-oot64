"""Checks that a release folder or zip contains no ROM, no data extracted from a ROM and nothing else that must
never be distributed. Exit code 0 = clean, 1 = something was found (each finding is printed).

    python tools/check_release.py dist/web
    python tools/check_release.py dist/ZMP-0.6.0-windows.zip

What it looks for:
  - ROM files by extension (.z64 .n64 .v64 .rom .ndd) and by content: the first four bytes of an N64 ROM in any of
    its three byte orders, whatever the file is called;
  - archives extracted from a ROM: oot.o2r, oot-mq.o2r (or any .o2r/.otr other than Ship of Harkinian's own soh.o2r);
  - inside an Emscripten data package (soh.data + soh.js), a packed oot.o2r / oot-mq.o2r;
  - debug symbols (.pdb) and harness/test leftovers that are not part of a player release;
  - any single file of 100 MB or more (the hosting limit, and no game file of ours is that big);
  - inside executables and WebAssembly, absolute paths of a user folder of the build machine (the Windows Users
    folder or /home/<name>/...), which libraries compiled from a cache on that machine leave behind: build releases on CI or
    with a fresh vcpkg (VCPKG_BINARY_SOURCES=clear) in a folder outside your user folder.
"""

from __future__ import annotations

import io
import re
import sys
import zipfile
from pathlib import Path

ROM_MAGICS = (b"\x80\x37\x12\x40", b"\x37\x80\x40\x12", b"\x40\x12\x37\x80")
ROM_EXTS = {".z64", ".n64", ".v64", ".rom", ".ndd"}
ALLOWED_O2R = {"soh.o2r"}
NOT_FOR_PLAYERS = {".pdb", ".ilk", ".exp"}
MAX_BYTES = 100 * 1024 * 1024
PACKED_ROM_DATA = re.compile(rb"oot(-mq)?\.o2r")
USER_PATH = re.compile(rb"[A-Za-z]:[\\/]Users[\\/][^\\/\x00]{1,64}[\\/]|/home/[a-z_][a-z0-9_-]{0,31}/")
BINARY_EXTS = {".exe", ".dll", ".wasm", ".js"}


def entries(target: Path):
    """(name, size, first 4 bytes, read-all callable) for every file of a folder or zip."""
    if target.is_dir():
        for p in sorted(target.rglob("*")):
            if p.is_file():
                with p.open("rb") as f:
                    head = f.read(4)
                yield p.relative_to(target).as_posix(), p.stat().st_size, head, p.read_bytes
    else:
        z = zipfile.ZipFile(target)
        for info in z.infolist():
            if info.is_dir():
                continue
            with z.open(info) as f:
                head = f.read(4)
            yield info.filename, info.file_size, head, (lambda i=info: z.read(i))


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    target = Path(sys.argv[1])
    findings = []
    count = 0
    for name, size, head, read in entries(target):
        count += 1
        base = name.rsplit("/", 1)[-1].lower()
        ext = "." + base.rsplit(".", 1)[-1] if "." in base else ""
        if ext in ROM_EXTS:
            findings.append(f"{name}: ROM file extension")
        if head in ROM_MAGICS:
            findings.append(f"{name}: starts like an N64 ROM")
        if ext in {".o2r", ".otr"} and base not in ALLOWED_O2R:
            findings.append(f"{name}: an archive extracted from a ROM ({base})")
        if ext in NOT_FOR_PLAYERS:
            findings.append(f"{name}: debug/build leftover ({ext})")
        if size >= MAX_BYTES:
            findings.append(f"{name}: {size / 1e6:.0f} MB, larger than 100 MB")
        if ext in BINARY_EXTS:
            m = USER_PATH.search(read())
            if m:
                findings.append(f"{name}: embeds a build machine's user folder path ({m.group(0)[:3].decode()}...)")
        if base == "soh.js" and PACKED_ROM_DATA.search(read()):
            # the loader lists the files packed into soh.data; the extractor's own code names oot.o2r as its output,
            # so only a packed entry ("filename":"/oot.o2r") counts
            if re.search(rb'"filename"\s*:\s*"/oot(-mq)?\.o2r"', read()):
                findings.append(f"{name}: soh.data packs an oot.o2r (ROM data)")
    print(f"check_release: {count} files in {target}")
    for f in findings:
        print("  FOUND " + f)
    print("clean" if not findings else f"{len(findings)} finding(s): do not publish")
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
