"""Writes a Windows icon resource as a COFF object that `go build` links into the executable (an .syso file).

    python tools/make_icon_syso.py branding-zmp/icono-zmp.ico server/cmd/zmp-server/rsrc_windows_amd64.syso

The object has one .rsrc section with the icon images (RT_ICON) and their group (RT_GROUP_ICON, id 1), and
ADDR32NB relocations for the data entries, the layout the Go linker reads. No tool of the Windows SDK is needed and
nothing of the machine that ran it ends up in the file (no paths, no time stamp), so the result is reproducible.
"""

from __future__ import annotations

import struct
import sys
from pathlib import Path

RT_ICON, RT_GROUP_ICON = 3, 14
LANG = 0x0409
IMAGE_FILE_MACHINE_AMD64 = 0x8664
IMAGE_REL_AMD64_ADDR32NB = 3


def read_ico(path: Path) -> tuple[list[bytes], list[tuple]]:
    d = path.read_bytes()
    reserved, kind, count = struct.unpack_from("<HHH", d, 0)
    if reserved != 0 or kind != 1 or count == 0:
        raise SystemExit(f"{path}: not an .ico file")
    images, entries = [], []
    for i in range(count):
        w, h, colors, res, planes, bits, size, offset = struct.unpack_from("<BBBBHHII", d, 6 + 16 * i)
        images.append(d[offset:offset + size])
        entries.append((w, h, colors, res, planes, bits, size))
    return images, entries


def directory(entries: list[tuple[int, int]]) -> bytes:
    """A resource directory with id entries: (id, offset | 0x80000000 for a subdirectory)."""
    out = struct.pack("<IIHHHH", 0, 0, 0, 0, 0, len(entries))
    for ident, offset in entries:
        out += struct.pack("<II", ident, offset)
    return out


def build(images: list[bytes], entries: list[tuple]) -> bytes:
    group = struct.pack("<HHH", 0, 1, len(images))
    for n, (w, h, colors, res, planes, bits, size) in enumerate(entries, start=1):
        group += struct.pack("<BBBBHHIH", w, h, colors, res, planes, bits, size, n)
    # (type, id, data) in the order of the tree: RT_ICON 1..n, then RT_GROUP_ICON 1
    leaves = [(RT_ICON, n, img) for n, img in enumerate(images, start=1)] + [(RT_GROUP_ICON, 1, group)]
    types = [RT_ICON, RT_GROUP_ICON]
    # Sizes: root, one name directory per type, one language directory per leaf, the data entries, then the data.
    root_size = 16 + 8 * len(types)
    name_sizes = {t: 16 + 8 * sum(1 for x in leaves if x[0] == t) for t in types}
    lang_size = 16 + 8
    pos = root_size
    name_off = {}
    for t in types:
        name_off[t] = pos
        pos += name_sizes[t]
    lang_off = []
    for _ in leaves:
        lang_off.append(pos)
        pos += lang_size
    entry_off = []
    for _ in leaves:
        entry_off.append(pos)
        pos += 16
    data_off = []
    for leaf in leaves:
        pos = (pos + 7) & ~7
        data_off.append(pos)
        pos += len(leaf[2])
    total = (pos + 7) & ~7

    sec = bytearray(total)
    sec[0:root_size] = directory([(t, name_off[t] | 0x80000000) for t in types])
    for t in types:
        items = [(leaf[1], lang_off[i] | 0x80000000) for i, leaf in enumerate(leaves) if leaf[0] == t]
        sec[name_off[t]:name_off[t] + name_sizes[t]] = directory(items)
    relocs = []
    for i, leaf in enumerate(leaves):
        sec[lang_off[i]:lang_off[i] + lang_size] = directory([(LANG, entry_off[i])])
        sec[entry_off[i]:entry_off[i] + 16] = struct.pack("<IIII", data_off[i], len(leaf[2]), 0, 0)
        relocs.append(entry_off[i])  # OffsetToData is an RVA: relocated against the section
        sec[data_off[i]:data_off[i] + len(leaf[2])] = leaf[2]

    header_size, section_header_size = 20, 40
    raw_ptr = header_size + section_header_size
    reloc_ptr = raw_ptr + len(sec)
    sym_ptr = reloc_ptr + 10 * len(relocs)
    out = struct.pack("<HHIIIHH", IMAGE_FILE_MACHINE_AMD64, 1, 0, sym_ptr, 1, 0, 0)
    out += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(sec), raw_ptr, reloc_ptr, 0, len(relocs), 0, 0x40000040)
    out += bytes(sec)
    for r in relocs:
        out += struct.pack("<IIH", r, 0, IMAGE_REL_AMD64_ADDR32NB)
    out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)  # the section's symbol (static)
    out += struct.pack("<I", 4)  # empty string table
    return out


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__)
        return 2
    images, entries = read_ico(Path(sys.argv[1]))
    Path(sys.argv[2]).write_bytes(build(images, entries))
    print(f"{sys.argv[2]}: {len(images)} icon images")
    return 0


if __name__ == "__main__":
    sys.exit(main())
