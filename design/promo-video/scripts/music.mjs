// Synthesises the soft background score into public/music.wav (no samples, no licensing).
// Sections line up with the scene timing in src/theme.ts.
import { writeFileSync } from "node:fs";

const RATE = 48000;
const SECONDS = 30;
const N = RATE * SECONDS;
const left = new Float32Array(N);
const right = new Float32Array(N);

const hz = (midi) => 440 * 2 ** ((midi - 69) / 12);

// D major: Dmaj7 – Bm7 – Gmaj7 – Asus2, three seconds each.
const chords = [
  { root: 38, notes: [62, 66, 69, 73] },
  { root: 35, notes: [62, 66, 69, 71] },
  { root: 31, notes: [62, 66, 67, 71] },
  { root: 33, notes: [61, 64, 69, 71] },
];
const CHORD_LEN = 3;

const add = (t0, dur, freq, amp, pan, { attack, release, partials, decay }) => {
  const start = Math.floor(t0 * RATE);
  const end = Math.min(N, Math.floor((t0 + dur + release) * RATE));
  const gl = Math.cos((pan * Math.PI) / 4 + Math.PI / 4);
  const gr = Math.sin((pan * Math.PI) / 4 + Math.PI / 4);
  for (let i = start; i < end; i++) {
    const t = (i - start) / RATE;
    let env = Math.min(1, t / attack);
    if (decay) env *= Math.exp(-t / decay);
    if (t > dur) env *= Math.max(0, 1 - (t - dur) / release);
    let s = 0;
    for (const [mult, a] of partials) s += a * Math.sin(2 * Math.PI * freq * mult * t);
    s *= amp * env;
    left[i] += s * gl;
    right[i] += s * gr;
  }
};

const pad = { attack: 1.2, release: 1.6, partials: [[1, 1], [1.003, 0.6], [2, 0.12]] };
const bass = { attack: 0.6, release: 1.2, partials: [[1, 1], [2, 0.25]] };
const keys = { attack: 0.006, release: 0.4, decay: 0.9, partials: [[1, 1], [2, 0.35], [3, 0.12], [4.01, 0.05]] };

// Ten chords cover 0–30 s; the last one rings into the fade.
for (let k = 0; k < 10; k++) {
  const chord = chords[k % 4];
  const t0 = k * CHORD_LEN;
  const last = k === 9;
  const dur = last ? 2.4 : CHORD_LEN;
  const chordNotes = last ? chords[0].notes : chord.notes;
  chordNotes.forEach((n, j) => add(t0, dur, hz(n), 0.035, j % 2 ? 0.35 : -0.35, pad));
  if (t0 >= 2) add(t0, dur, hz(last ? chords[0].root : chord.root), 0.06, 0, bass);
}

// Arpeggio runs through the four feature scenes (2–25 s).
const pattern = [0, 2, 1, 3, 2, 1];
for (let t = 2; t < 25; t += 0.5) {
  const chord = chords[Math.floor(t / CHORD_LEN) % 4];
  const step = Math.round(t * 2) % pattern.length;
  const note = chord.notes[pattern[step]] + 12;
  add(t, 0.45, hz(note), 0.06, (step / pattern.length) * 1.2 - 0.6, keys);
}
// Outro: a slow rising Dmaj9 figure under the logo.
[74, 78, 81, 85, 88].forEach((n, i) => add(25.2 + i * 0.45, 1.2, hz(n), 0.055, -0.4 + i * 0.2, { ...keys, decay: 1.8 }));

// Light stereo reverb: a few feedback combs per channel.
const reverb = (buf, delays) => {
  const wet = new Float32Array(N);
  for (const d of delays) {
    const D = Math.floor(d * RATE);
    const line = new Float32Array(N);
    for (let i = 0; i < N; i++) line[i] = buf[i] + (i >= D ? line[i - D] * 0.55 : 0);
    for (let i = 0; i < N; i++) wet[i] += (i >= D ? line[i - D] : 0) / delays.length;
  }
  for (let i = 0; i < N; i++) buf[i] = buf[i] * 0.8 + wet[i] * 0.35;
};
reverb(left, [0.0297, 0.0371, 0.0411, 0.0437]);
reverb(right, [0.0313, 0.0353, 0.0423, 0.0449]);

// Normalise to −3 dBFS peak and write 16-bit stereo WAV.
let peak = 0;
for (let i = 0; i < N; i++) peak = Math.max(peak, Math.abs(left[i]), Math.abs(right[i]));
const gain = 0.707 / peak;
const out = Buffer.alloc(44 + N * 4);
out.write("RIFF", 0);
out.writeUInt32LE(36 + N * 4, 4);
out.write("WAVEfmt ", 8);
out.writeUInt32LE(16, 16);
out.writeUInt16LE(1, 20);
out.writeUInt16LE(2, 22);
out.writeUInt32LE(RATE, 24);
out.writeUInt32LE(RATE * 4, 28);
out.writeUInt16LE(4, 32);
out.writeUInt16LE(16, 34);
out.write("data", 36);
out.writeUInt32LE(N * 4, 40);
for (let i = 0; i < N; i++) {
  out.writeInt16LE(Math.round(left[i] * gain * 32767), 44 + i * 4);
  out.writeInt16LE(Math.round(right[i] * gain * 32767), 46 + i * 4);
}
writeFileSync(new URL("../public/music.wav", import.meta.url), out);
console.log(`wrote public/music.wav, peak gain ${gain.toFixed(2)}`);
