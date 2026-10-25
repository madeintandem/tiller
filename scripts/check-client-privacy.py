#!/usr/bin/env python3
"""Keep maintained files anonymous without scanning or rewriting Git history."""

import functools
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
from typing import NamedTuple

# Issue #84 accepts neutral project shapes and public domain/form nouns. Do not
# broaden this to product terms, all personal names, or historical Git objects.
#
# The denylist holds SHA-256 digests of lowercased text, so the public tree never
# spells what it guards. Hash a new entry locally and add only the digest:
#   python3 -c 'import hashlib,sys; print(hashlib.sha256(sys.argv[1].lower().encode()).hexdigest())' 'text'
# A substring entry is stored as a (length, digest) tuple: add only the length
# and the digest, never the text.
# Short words can still be confirmed by guessing. This keeps the names out of
# casual reading, search and indexing, not away from a determined guesser.


class Denylist(NamedTuple):
    substrings: frozenset  # (length, digest): a project name anywhere inside a word
    words: frozenset  # digest: a project name matched only as a whole word
    pairs: frozenset  # digest of "first last": a personal name as two adjacent words


DENYLIST = Denylist(
    substrings=frozenset({
        (6, "5cbe46195beeb4e6e16f2100efbdb38bbcad5bce931f04d61126a73978e3682b"),
        (9, "5c8a89bb4958f63376d65024207cec1728ff59b74734304b5be21c42440626b3"),
    }),
    words=frozenset({
        "4f0a1f5aef53e953cc28e5086b98eca5d40c8820325fce6a33f34b22c84bbefd",
        "e2be86b74d5544878ce5285d45bb4d998fb83f8c5ddd56eaea15787112b4a000",
        "918bdc1ec8097cf1445933826216b35411419b91cfade684fa680bed56780e8d",
        "ec47860233d36cad00db3ed18c8ed7d57690cb8689caa86cce93ef75d767e7f1",
    }),
    pairs=frozenset({
        "9fcff232b96c68129abb7b5cb43a564e3c0970fe4788419ee3f55e194a5b85cc",
        "208da4bcf655b0d83f85fe062f4d31e75e791509f594a0ffa89ac9d401ea7786",
        "dc78bdb4113c541f1ac10c18866371f1a617adef11ed8ed9ffd324d446fc198d",
    }),
)

WORD = re.compile(r"[a-z0-9]+")


@functools.lru_cache(maxsize=65536)
def digest(text):
    return hashlib.sha256(text.encode()).hexdigest()


def identifying(text, denylist):
    """Report whether text names a denylisted project or person.

    Words are lowercase alphanumeric runs, so paths, URLs, hyphens and
    underscores split cleanly. A substring entry matches anywhere inside a
    word ("getNAMEUser"); a word entry matches only the whole word.
    """
    words = WORD.findall(text.lower())
    lengths = sorted({length for length, _ in denylist.substrings})
    for index, word in enumerate(words):
        if digest(word) in denylist.words:
            return True
        if any((length, digest(word[start:start + length])) in denylist.substrings
               for length in lengths for start in range(len(word) - length + 1)):
            return True
        if index + 1 < len(words) and digest(word + " " + words[index + 1]) in denylist.pairs:
            return True
    return False


def scan(root, denylist):
    """Return findings for the current tree: tracked, new, and deleted files."""
    findings = []
    paths = subprocess.check_output(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"], cwd=root,
    ).split(b"\0")
    for raw in sorted(set(paths)):
        if not raw:
            continue
        name = os.fsdecode(raw)
        if identifying(name, denylist):
            findings.append(f"identifying path: {name}")
        path = root / name
        try:
            data = os.fsencode(os.readlink(path)) if path.is_symlink() else path.read_bytes()
        except FileNotFoundError:
            # Tracked deletions are still listed until staged.
            continue
        if b"\0" in data:
            continue
        for number, line in enumerate(data.decode("utf-8", errors="replace").splitlines(), 1):
            if identifying(line, denylist):
                # Report the coordinate, not potentially private engagement text.
                findings.append(f"identifying text: {name}:{number}")
    return findings


# Stand-in entries that exercise every matching rule without the real
# denylist's plaintext ever appearing in the tree.
SAMPLE = Denylist(
    substrings=frozenset({(len("zorbex"), digest("zorbex"))}),
    words=frozenset({digest("quill")}),
    pairs=frozenset({digest("ada quentin")}),
)


def self_test():
    for entry in (*DENYLIST.words, *DENYLIST.pairs, *(value for _, value in DENYLIST.substrings)):
        assert re.fullmatch(r"[0-9a-f]{64}", entry), "denylist entries must be SHA-256 digests"
    for text in ("zorbex", "ZORBEX", "https://github.com/example/zorbex-platform",
                 "~/dev/zorbexplatform/state.yml", "zorbex_cache", "quill", "Quill's notes",
                 "~/dev/quill/state.yml", "Ada Quentin", "ADA  QUENTIN", "ada-quentin",
                 "oldzorbex", "getZorbexUser", "myzorbexplatform"):
        assert identifying(text, SAMPLE), repr(text)
    for text in ("Acme Platform", "Lantern", "AHJ", "Phase Submission", "Device Readiness",
                 "JC Grubbs", "Ada Lovelace", "Bob Quentin", "quills", "requill",
                 "https://github.com/madeintandem/tiller"):
        assert not identifying(text, SAMPLE), repr(text)
    # Scan an isolated current tree: tracked files, new files, filenames,
    # deletions, and accepted domain nouns.
    with tempfile.TemporaryDirectory(prefix="tiller-privacy-") as directory:
        root = Path(directory)
        subprocess.run(["git", "init", "--quiet", str(root)], check=True)
        fixture = root / "fixture.md"
        fixture.write_text("Acme Platform, Lantern, AHJ, Phase Submission, Device Readiness\n")
        subprocess.run(["git", "add", "fixture.md"], cwd=root, check=True)
        assert scan(root, SAMPLE) == []
        fixture.write_text("Private project: ZORBEX\n")
        assert scan(root, SAMPLE) == ["identifying text: fixture.md:1"]
        fixture.unlink()
        assert scan(root, SAMPLE) == []
        private_path = root / "zorbex-platform.md"
        private_path.write_text("otherwise neutral\n")
        assert scan(root, SAMPLE) == ["identifying path: zorbex-platform.md"]
        private_path.unlink()
        new_file = root / "new.md"
        new_file.write_text("Ada Quentin\n")
        assert scan(root, SAMPLE) == ["identifying text: new.md:1"]
        new_file.unlink()
        # The real command over the same tree: shipped denylist, exit status,
        # and the root derived from the script's own location.
        scripts = root / "scripts"
        scripts.mkdir()
        guard = scripts / Path(__file__).name
        guard.write_bytes(Path(__file__).read_bytes())
        result = subprocess.run([sys.executable, str(guard)], capture_output=True, text=True)
        assert result.returncode == 0, result.stderr
    print("check-client-privacy: matching rules, accepted stand-ins, and current-tree scanning passed")


def main():
    if sys.argv[1:] == ["--self-test"]:
        self_test()
        return 0
    if sys.argv[1:]:
        print("usage: check-client-privacy.py [--self-test]", file=sys.stderr)
        return 2
    findings = scan(Path(__file__).resolve().parent.parent, DENYLIST)
    for finding in findings:
        print(f"check-client-privacy: {finding}", file=sys.stderr)
    if findings:
        print("Use Acme/Lantern; remove private engagement links and coordinates. "
              "Do not rewrite Git history.", file=sys.stderr)
        return 1
    print("check-client-privacy: maintained tree is anonymous")
    return 0


if __name__ == "__main__":
    sys.exit(main())
