#!/usr/bin/env python3
"""Stamp the release version onto the pages/ landing page before deploy.

The version is written on the page in three places: the header badge, the hero
download button, and the footer. Doing that by hand at release time is how the
page came to advertise v0.6.8 next to a download button that served v0.7.3, so
the number is now derived from the release itself. The checked-in literal stays
behind as a readable fallback for anyone opening the file straight from disk.

Run by .github/workflows/pages.yml; can also be run locally:

    RELEASE_TAG=v0.7.4 python3 .github/scripts/stamp-version.py pages

Exits non-zero if an anchor it is supposed to stamp is missing, so a reformat
of the page fails the deploy instead of silently shipping a stale number.
"""

import json
import os
import pathlib
import re
import sys
import urllib.request
from typing import List, Optional

# (pattern, replacement count) — each must match exactly once.
ANCHORS = [
    (r'(<span class="brand-ver">)v?\d+\.\d+\.\d+(</span>)', 1),
    (r'(<span data-en="Download )v?\d+\.\d+\.\d+(">下载 )v?\d+\.\d+\.\d+(</span>)', 1),
    (r'(<p class="mono">)v?\d+\.\d+\.\d+( · 2026</p>)', 1),
]


def normalise(tag: str) -> str:
    tag = tag.strip()
    if not re.fullmatch(r"v?\d+\.\d+\.\d+", tag):
        raise ValueError("not a plain vX.Y.Z version tag: %r" % tag)
    return tag if tag.startswith("v") else "v" + tag


def set_output(key: str, value: str) -> None:
    """Hand the version to the next workflow step (it verifies the live page)."""
    path = os.environ.get("GITHUB_OUTPUT", "").strip()
    if not path:
        return
    with open(path, "a", encoding="utf-8") as handle:
        handle.write("%s=%s\n" % (key, value))


def version_on_page(html: str) -> Optional[str]:
    """The version already written in the header badge, for the fallback case."""
    found = re.search(r'<span class="brand-ver">(v?\d+\.\d+\.\d+)</span>', html)
    return found.group(1) if found else None


def release_tag() -> Optional[str]:
    """The version to show: this run's release tag, else the latest release."""
    tag = os.environ.get("RELEASE_TAG", "").strip()
    if tag:
        return tag

    repo = os.environ.get("GITHUB_REPOSITORY", "").strip()
    if not repo:
        return None
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN") or ""
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "beacon-pages"}
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(
        "https://api.github.com/repos/%s/releases/latest" % repo, headers=headers
    )
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.load(response)["tag_name"]
    except Exception as error:  # a lookup hiccup must not block a deploy
        # Never a quiet fallback: the page then keeps whatever number it has,
        # which is exactly how it ended up advertising an old release. Shout.
        print(
            "::warning title=Version not checked::could not read the latest release (%s) — "
            "the page keeps the version already written in it; check it by hand" % error
        )
        return None


def main(argv: List[str]) -> int:
    root = pathlib.Path(argv[1] if len(argv) > 1 else "pages")
    page = root / "index.html"
    html = page.read_text(encoding="utf-8")

    tag = release_tag()
    if not tag:
        current = version_on_page(html)
        print("::warning title=Version not checked::no release tag available; %s keeps %s" % (page, current or "the version it has"))
        if current:
            set_output("version", current)
        return 0

    version = normalise(tag)
    print("stamping %s onto %s" % (version, page))

    for pattern, expected in ANCHORS:
        html, count = re.subn(pattern, lambda m: version.join(m.groups()), html)
        # group(0) shape: prefix + version + [version +] suffix; join() above
        # rebuilds it with the new version in each version slot.
        if count != expected:
            print(
                "expected %d match(es) of %s, found %d — the page changed shape, fix the stamp"
                % (expected, pattern, count)
            )
            return 1

    page.write_text(html, encoding="utf-8")
    set_output("version", version)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
