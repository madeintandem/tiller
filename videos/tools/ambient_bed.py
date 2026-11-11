#!/usr/bin/env python3
"""Synthesize the ambient music bed that sits under each video's opening and closing.

A slow pad in D major: four chords, twelve seconds each, every voice a few detuned
sines that swell and recede, with a sparse bell line on top and a little air. It is
written as an exact loop (every pitch and every swell completes whole cycles in LOOP
seconds, and the bells and the reverb tail wrap around the end), because the build
loops the bed for the length of the program and opens a window on it wherever the
outro happens to fall.

Nothing is sampled, so there is nothing to license: the track is this file's output.

usage: uv run --with numpy tools/ambient_bed.py <out.wav>
"""
import sys
import wave

import numpy as np

SR = 48000
LOOP = 48.0
N = int(SR * LOOP)
t = np.arange(N) / SR
rng = np.random.default_rng(7)


def hz(note: float) -> float:
    """MIDI note to frequency, snapped to a whole number of cycles per loop."""
    return round(440.0 * 2 ** ((note - 69) / 12) * LOOP) / LOOP


def swell(cycles: int, phase: float, floor: float = 0.0) -> np.ndarray:
    """A slow rise and fall that completes `cycles` whole turns in the loop."""
    return floor + (1 - floor) * (0.5 - 0.5 * np.cos(2 * np.pi * (cycles * t / LOOP + phase)))


def window(start: float, length: float, fade: float) -> np.ndarray:
    """1 inside [start, start+length), with cosine fades either side that wrap around the loop."""
    # The fade-in starts `fade` before `start`, so neighbours overlap and the level holds.
    pos_in = (t - (start - fade)) % LOOP
    up = np.clip(pos_in / fade, 0, 1)
    down = np.clip((length + fade - pos_in) / fade, 0, 1)
    return (0.5 - 0.5 * np.cos(np.pi * up)) * (0.5 - 0.5 * np.cos(np.pi * down))


def voice(note: float, gain: float, pan: float) -> np.ndarray:
    """One pad voice: three slightly detuned sines and a soft octave, each breathing on its own."""
    out = np.zeros((N, 2))
    f = hz(note)
    for k, cents in enumerate((-6, 0, 7)):
        fk = round(f * 2 ** (cents / 1200) * LOOP) / LOOP
        tone = np.sin(2 * np.pi * fk * t + rng.uniform(0, 2 * np.pi))
        tone += 0.18 * np.sin(2 * np.pi * 2 * fk * t + rng.uniform(0, 2 * np.pi))
        tone *= swell(int(rng.integers(3, 8)), rng.uniform(), floor=0.45)
        p = np.clip(pan + (k - 1) * 0.35, -1, 1)
        out[:, 0] += tone * np.sqrt((1 - p) / 2)
        out[:, 1] += tone * np.sqrt((1 + p) / 2)
    return out * gain / 3


# Dadd9, Bm7, Gmaj7, Asus4 resolving home: bass, then the chord, low to high.
CHORDS = [
    (38, [50, 57, 64, 66, 69]),
    (35, [50, 57, 62, 66, 69]),
    (31, [50, 55, 59, 66, 71]),
    (33, [52, 57, 62, 64, 69]),
]
SPAN = LOOP / len(CHORDS)
FADE = 4.0

mix = np.zeros((N, 2))
for i, (bass, notes) in enumerate(CHORDS):
    env = window(i * SPAN, SPAN, FADE)[:, None]
    chord = voice(bass, 0.9, 0.0) + voice(bass + 12, 0.5, 0.0)
    for j, note in enumerate(notes):
        chord += voice(note, 0.55 - 0.05 * j, (-0.7, 0.5, -0.3, 0.7, 0.0)[j])
    mix += chord * env

# A sparse bell line: one soft note every few seconds from the chord of the moment,
# an octave up, with a long decay that wraps around the end of the loop.
bells = np.zeros((N, 2))
at = 1.5
while at < LOOP - 0.5:
    bass, notes = CHORDS[int(at // SPAN) % len(CHORDS)]
    f = hz(int(rng.choice(notes[1:])) + 12)
    pos = (t - at) % LOOP
    ring = np.exp(-pos / 2.6) * np.clip(pos / 0.012, 0, 1)
    tone = (np.sin(2 * np.pi * f * pos) + 0.3 * np.sin(2 * np.pi * 2.01 * f * pos) * np.exp(-pos / 0.9)) * ring
    p = rng.uniform(-0.6, 0.6)
    bells[:, 0] += tone * np.sqrt((1 - p) / 2)
    bells[:, 1] += tone * np.sqrt((1 + p) / 2)
    at += rng.uniform(2.6, 4.4)
mix += bells * 0.16

# Air: quiet band-limited noise, built in the frequency domain so it loops too.
spectrum = np.fft.rfft(rng.standard_normal((N, 2)), axis=0)
freqs = np.fft.rfftfreq(N, 1 / SR)
spectrum *= (np.exp(-(((freqs - 2600) / 1800) ** 2)))[:, None]
air = np.fft.irfft(spectrum, n=N, axis=0)
mix += air / np.abs(air).max() * 0.035 * swell(5, 0.3, floor=0.5)[:, None]

# A long, dark reverb, convolved circularly so the tail wraps instead of being cut.
tail = int(SR * 5.0)
decay = np.exp(-np.arange(tail) / (SR * 1.4))
impulse = np.zeros((N, 2))
impulse[:tail] = rng.standard_normal((tail, 2)) * decay[:, None]
shape = np.exp(-freqs / 3200)[:, None]  # darker as it goes up
wet = np.fft.irfft(np.fft.rfft(mix, axis=0) * np.fft.rfft(impulse, axis=0) * shape, n=N, axis=0)
mix = mix / np.abs(mix).max() * 0.6 + wet / np.abs(wet).max() * 0.55

# Roll off the top so nothing sits where the voice's consonants are, then set the level.
spectrum = np.fft.rfft(mix, axis=0)
spectrum *= (1 / np.sqrt(1 + (freqs / 5200) ** 4))[:, None]
spectrum[freqs < 28] = 0
mix = np.fft.irfft(spectrum, n=N, axis=0)
mix = mix / np.abs(mix).max() * 0.7

with wave.open(sys.argv[1], "wb") as out:
    out.setnchannels(2)
    out.setsampwidth(2)
    out.setframerate(SR)
    out.writeframes((mix * 32767).astype("<i2").tobytes())
print(f"wrote {sys.argv[1]}: {LOOP:g} s loop")
