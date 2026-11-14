"""Builds the zmp-server release binaries (static, no dependencies) into dist/.

    python tools/build_server.py --version 0.6.2

Runs the server's tests first, then cross-compiles for Windows x64, Linux x64 and Linux ARM64 (Raspberry Pi 4/5,
ARM VPS). Needs Go 1.26 or newer.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TARGETS = [("windows", "amd64", ".exe"), ("linux", "amd64", ""), ("linux", "arm64", "")]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--version", required=True)
    ap.add_argument("--out", type=Path, default=ROOT / "dist")
    ap.add_argument("--skip-tests", action="store_true")
    args = ap.parse_args()
    server = ROOT / "server"
    args.out.mkdir(parents=True, exist_ok=True)
    if not args.skip_tests:
        if subprocess.run(["go", "test", "./..."], cwd=server).returncode != 0:
            print("server tests failed: nothing built", file=sys.stderr)
            return 1
    lines = []
    for goos, goarch, ext in TARGETS:
        name = f"zmp-server-{args.version}-{goos}-{goarch}{ext}"
        env = dict(os.environ, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0")
        cmd = ["go", "build", "-trimpath", "-ldflags", f"-s -w -X main.version={args.version}",
               "-o", str(args.out / name), "./cmd/zmp-server"]
        if subprocess.run(cmd, cwd=server, env=env).returncode != 0:
            print(f"build failed: {name}", file=sys.stderr)
            return 1
        sha = hashlib.sha256((args.out / name).read_bytes()).hexdigest()
        lines.append(f"{sha}  {name}")
        print(f"{args.out / name} ({(args.out / name).stat().st_size / 1e6:.1f} MB)")
    (args.out / f"zmp-server-{args.version}.sha256.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
