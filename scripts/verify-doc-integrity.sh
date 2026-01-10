#!/usr/bin/env bash
set -euo pipefail

# Checks repo-local markdown links resolve and that key version claims match go.mod.
#
# Scope:
# - README.md
# - docs/**/*.md

python3 - <<'PY'
from __future__ import annotations

import os
import re
import sys
from pathlib import Path


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8", errors="replace")


def iter_md_files(repo_root: Path) -> list[Path]:
    files: list[Path] = []
    readme = repo_root / "README.md"
    if readme.exists():
        files.append(readme)
    docs_dir = repo_root / "docs"
    if docs_dir.exists():
        files.extend(sorted(docs_dir.rglob("*.md")))
    return files


LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


def is_external(link: str) -> bool:
    link = link.strip()
    if not link:
        return True
    lowered = link.lower()
    return (
        lowered.startswith("http://")
        or lowered.startswith("https://")
        or lowered.startswith("mailto:")
        or lowered.startswith("#")
        or lowered.startswith("data:")
    )


def normalize_link_target(raw: str) -> str:
    # Strip optional title: (path "title") or (path 'title')
    raw = raw.strip()
    if raw.startswith("<") and raw.endswith(">"):
        raw = raw[1:-1].strip()
    # Split off title if present.
    # This is a conservative split: first whitespace ends the URL.
    raw = raw.split()[0]
    # Remove fragment/query when checking file existence.
    raw = raw.split("#", 1)[0]
    raw = raw.split("?", 1)[0]
    return raw.strip()


def check_links(repo_root: Path) -> list[str]:
    problems: list[str] = []
    for md in iter_md_files(repo_root):
        content = read_text(md)
        for m in LINK_RE.finditer(content):
            raw = m.group(1)
            if is_external(raw):
                continue
            target = normalize_link_target(raw)
            if not target or target == ".":
                continue

            # Treat absolute repo paths as relative to repo root.
            if target.startswith("/"):
                rel = target.lstrip("/")
                resolved = repo_root / rel
            else:
                resolved = (md.parent / target).resolve()

            # Only enforce links that resolve inside the repo.
            try:
                resolved.relative_to(repo_root.resolve())
            except ValueError:
                continue

            if not resolved.exists():
                problems.append(f"{md}: broken link target '{raw}' -> '{resolved.relative_to(repo_root)}'")

    return problems


def parse_go_version(repo_root: Path) -> str | None:
    go_mod = repo_root / "go.mod"
    if not go_mod.exists():
        return None
    for line in read_text(go_mod).splitlines():
        if line.startswith("go "):
            return line.split()[1].strip()
    return None


def check_go_version_badge(repo_root: Path) -> list[str]:
    problems: list[str] = []
    readme = repo_root / "README.md"
    if not readme.exists():
        return problems

    go_ver = parse_go_version(repo_root)
    if not go_ver:
        return problems

    content = read_text(readme)
    # Example: https://img.shields.io/badge/go-1.21+-blue.svg
    m = re.search(r"img\.shields\.io/badge/go-([0-9]+\.[0-9]+)\+", content)
    if not m:
        return problems

    badge_ver = m.group(1)
    if badge_ver != go_ver:
        problems.append(
            f"README.md: Go version badge claims {badge_ver}+ but go.mod declares go {go_ver}"
        )

    return problems


def main() -> int:
    repo_root = Path(os.getcwd())
    problems: list[str] = []
    problems.extend(check_links(repo_root))
    problems.extend(check_go_version_badge(repo_root))

    if problems:
        print("doc-integrity: FAIL")
        for p in problems:
            print(f"- {p}")
        return 1

    print("doc-integrity: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
PY

