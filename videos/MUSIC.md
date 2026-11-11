# Music

The bed under each video's opening and closing slides.

- **Track:** an ambient pad in D major, 48 seconds, written as an exact loop.
- **Source:** synthesized by `tools/ambient_bed.py` in this repo. Nothing is sampled.
- **License:** none needed. The track is that script's output and belongs to Made In Tandem.
- **In use since:** 2026-11-11, for videos 1 and 2.

To regenerate it:

```sh
uv run --with numpy tools/ambient_bed.py bed.wav
ffmpeg -i bed.wav -c:a libmp3lame -b:a 256k .local/media/music.mp3
```

The script draws its random choices from a fixed seed, so a rerun produces the same track.
