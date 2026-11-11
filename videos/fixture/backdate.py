#!/usr/bin/env python3
"""Spread a freshly built fixture's history over the weeks before now.

A fixture script runs a project's whole life in a few seconds, so every event
carries the same minute and the console's Report page draws one vertical step.
This rewrites every timestamp under .tiller/ so the events land evenly across
the last DAYS days, in their original order, and moves each event file to the
date folder its new time names. Nothing else in a file changes.

usage: backdate.py <project-dir> [days]
"""
import re
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

STAMP = re.compile(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z")


def parse(s: str) -> datetime:
    head, _, frac = s[:-1].partition(".")
    t = datetime.strptime(head, "%Y-%m-%dT%H:%M:%S").replace(tzinfo=timezone.utc)
    return t + timedelta(seconds=float("0." + frac) if frac else 0)


def render(t: datetime) -> str:
    frac = f"{t.microsecond:06d}".rstrip("0")
    return t.strftime("%Y-%m-%dT%H:%M:%S") + (f".{frac}" if frac else "") + "Z"


def main() -> None:
    root = Path(sys.argv[1]) / ".tiller"
    days = float(sys.argv[2]) if len(sys.argv) > 2 else 40
    files = [p for p in root.rglob("*") if p.is_file() and p.suffix in {".yml", ".yaml", ".md", ".json", ".jsonl"}]
    texts = {p: p.read_text() for p in files}
    stamps = sorted({s for text in texts.values() for s in STAMP.findall(text)}, key=parse)
    if len(stamps) < 2:
        sys.exit("backdate: fewer than two timestamps under .tiller/")
    # Ranked, not scaled: a script's pauses are not a project's, so each distinct
    # moment takes the next even step. The last one lands an hour ago.
    end = datetime.now(timezone.utc).replace(microsecond=0) - timedelta(hours=1)
    step = timedelta(days=days) / (len(stamps) - 1)
    moved = {s: render(end - step * (len(stamps) - 1 - i)) for i, s in enumerate(stamps)}
    for p, text in texts.items():
        new = STAMP.sub(lambda m: moved[m.group(0)], text)
        if new != text:
            p.write_text(new)
    events = root / "events"
    for p in list(events.rglob("EV-*.yml")):
        at = re.search(r"^at: (\S+)$", p.read_text(), re.M)
        if not at:
            continue
        t = parse(at.group(1))
        dest = events / f"{t.year:04d}" / f"{t.month:02d}" / f"{t.day:02d}" / p.name
        if dest != p:
            dest.parent.mkdir(parents=True, exist_ok=True)
            p.rename(dest)
    for d in sorted((d for d in events.rglob("*") if d.is_dir()), reverse=True):
        if not any(d.iterdir()):
            d.rmdir()
    print(f"backdated {len(stamps)} moments across {days:g} days")


main()
