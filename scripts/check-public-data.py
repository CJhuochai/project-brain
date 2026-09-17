#!/usr/bin/env python3
"""Check tracked text for common public-data leaks without printing secret values.
This is a heuristic guard, not a security audit or a Git-history scrubber.
"""
import re
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RULES = {
    "credential": re.compile(r"gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,}|AKIA[A-Z0-9]{16}|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|sk-[A-Za-z0-9_-]{32,}"),
    "personal email": re.compile(r"[A-Za-z0-9._%+-]+@(?:qq|163|126)\.com", re.I),
    "personal local path": re.compile(r"[A-Z]:[\\/]+Users[\\/]+[0-9]{4,}", re.I),
}


def main():
    paths = subprocess.check_output(["git", "-C", str(ROOT), "ls-files", "-z"]).decode("utf8").split("\0")
    failures = []
    for name in filter(None, paths):
        path = ROOT / name
        if not path.is_file():
            continue
        data = path.read_bytes()
        if b"\0" in data:
            continue
        try:
            text = data.decode("utf-8-sig")
        except UnicodeDecodeError:
            failures.append((name, "invalid UTF-8"))
            continue
        if "\ufffd" in text or any(ord(c) < 32 and c not in "\n\r\t" for c in text):
            failures.append((name, "unexpected text character"))
        for kind, pattern in RULES.items():
            if pattern.search(text):
                failures.append((name, kind))
    for name, kind in failures:
        print(f"{name}: {kind}")
    print(f"Public-data text check: {len(failures)} finding(s)")
    return bool(failures)


if __name__ == "__main__":
    raise SystemExit(main())
