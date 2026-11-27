// Synthesises an original 30-second instrumental backing score into public/music.wav.
// Zero external samples or third-party assets; 100% deterministic Web/Node DSP synthesis.
// Tempo: 120 BPM (0.50 s/beat, 0.125 s/16th), aligned to the exact scene cuts in src/theme.ts:
//   0.00 s Intro    |  2.50 s Fill   |  8.50 s Generate
//  14.00 s Sync     | 19.50 s Vault  | 25.00 s Outro  | 30.00 s End
import { writeFileSync } from "node:fs";

const RATE = 48000;
const SECONDS = 30;
const N = RATE * SECONDS;
const TWO_PI = Math.PI * 2;
const BEAT = 0.5; // 120 BPM
const SIXTEENTH = 0.125;

// Deterministic PRNG (Mulberry32) so every run produces bit-identical output.
let seed = 0x50617373; // "Pass"
const rand = () => {
  seed |= 0;
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
};
const randSigned = () => rand() * 2 - 1;

const hz = (midi) => 440 * 2 ** ((midi - 69) / 12);
const clamp = (x, lo, hi) => (x < lo ? lo : x > hi ? hi : x);

// Dedicated stereo buses so bass/kick stay tight and dry while keys/pads bloom in space.
const makeBus = () => ({ L: new Float32Array(N), R: new Float32Array(N) });
const padBus = makeBus();
const bassBus = makeBus();
const keysBus = makeBus();
const arpBus = makeBus();
const percBus = makeBus();
const fxBus = makeBus();

// Sidechain envelope driven by kick hits (1.0 = unattenuated, dips smoothly on kick).
const sidechain = new Float32Array(N).fill(1);
const registerKickDuck = (t0, depth = 0.28, releaseSec = 0.16) => {
  const start = Math.floor(t0 * RATE);
  const attackSamples = Math.floor(0.006 * RATE);
  const releaseSamples = Math.floor(releaseSec * RATE);
  for (let k = 0; k < attackSamples + releaseSamples; k++) {
    const idx = start + k;
    if (idx < 0 || idx >= N) continue;
    let duck;
    if (k < attackSamples) {
      duck = (k / attackSamples) * depth;
    } else {
      const p = (k - attackSamples) / releaseSamples;
      duck = depth * Math.exp(-p * 3.8);
    }
    const mult = 1 - duck;
    if (mult < sidechain[idx]) sidechain[idx] = mult;
  }
};

// Equal-power stereo pan helper (-1 = left, 0 = center, +1 = right).
const panGains = (pan) => {
  const angle = (clamp(pan, -1, 1) * Math.PI) / 4 + Math.PI / 4;
  return [Math.cos(angle), Math.sin(angle)];
};

// ============================================================================
// 1. SYNTHESIS VOICES
// ============================================================================

// Warm analog-style polysynth pad voice with slow lowpass filter contour.
const addPadChord = (t0, dur, notes, amp, { cutoffStart = 900, cutoffEnd = 1500, attack = 0.65, release = 1.35 } = {}) => {
  const start = Math.max(0, Math.floor(t0 * RATE));
  const totalDur = dur + release;
  const end = Math.min(N, Math.floor((t0 + totalDur) * RATE));
  const len = end - start;
  if (len <= 0) return;

  const voiceCount = notes.length;
  for (let v = 0; v < voiceCount; v++) {
    const f0 = hz(notes[v]);
    const pan = voiceCount === 1 ? 0 : ((v / (voiceCount - 1)) * 2 - 1) * 0.58;
    const [gl, gr] = panGains(pan);
    const detune = 1 + (v % 2 === 0 ? 0.0024 : -0.0024);
    const phase0 = rand() * TWO_PI;
    const phase1 = rand() * TWO_PI;
    const lfoPhase = rand() * TWO_PI;

    // 1-pole resonant-like cascaded lowpass state (2-pole gentle slope).
    let lp1 = 0;
    let lp2 = 0;

    for (let k = 0; k < len; k++) {
      const t = k / RATE;
      let env = t < attack ? 0.5 - 0.5 * Math.cos((Math.PI * t) / attack) : 1;
      if (t > dur) {
        const rt = (t - dur) / release;
        env *= Math.max(0, 0.5 + 0.5 * Math.cos(Math.PI * Math.min(1, rt)));
      }

      // Subtle slow pitch warmth.
      const vib = 1 + 0.0012 * Math.sin(TWO_PI * 0.35 * t + lfoPhase);
      const w1 = TWO_PI * f0 * vib * t + phase0;
      const w2 = TWO_PI * f0 * detune * vib * t + phase1;

      // Band-limited warm saw/triangle hybrid (5 harmonics).
      let s =
        0.55 * Math.sin(w1) +
        0.42 * Math.sin(w2) +
        0.24 * Math.sin(2 * w1) +
        0.16 * Math.sin(2 * w2) +
        0.11 * Math.sin(3 * w1) +
        0.06 * Math.sin(4 * w1) +
        0.03 * Math.sin(5 * w1);

      const prog = Math.min(1, t / Math.max(0.05, dur));
      const cutoff = cutoffStart + (cutoffEnd - cutoffStart) * prog;
      const alpha = clamp((TWO_PI * cutoff) / (TWO_PI * cutoff + RATE), 0.005, 0.65);
      lp1 += alpha * (s - lp1);
      lp2 += alpha * (lp1 - lp2);

      const out = lp2 * env * (amp / Math.sqrt(voiceCount));
      const idx = start + k;
      padBus.L[idx] += out * gl;
      padBus.R[idx] += out * gr;
    }
  }
};

// Tactile felt-piano / glassy FM-Rhodes hybrid for the signature Passpine motif.
const addKeysNote = (t0, dur, midi, amp, pan = 0, { brightness = 1.0, decay = 1.15, release = 0.55 } = {}) => {
  const start = Math.max(0, Math.floor(t0 * RATE));
  const end = Math.min(N, Math.floor((t0 + dur + release) * RATE));
  const len = end - start;
  if (len <= 0) return;

  const f0 = hz(midi);
  const [gl, gr] = panGains(pan);
  const attack = 0.0045;
  const modRatio = 2.0;
  const bellRatio = 3.005;

  let clickLp = 0;
  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    let env = t < attack ? t / attack : 1;
    env *= Math.exp(-t / decay);
    if (t > dur) {
      env *= Math.max(0, 1 - (t - dur) / release);
    }

    // Gentle FM bell tine on attack that melts into warm fundamental.
    const fmIndex = 0.55 * brightness * Math.exp(-t / 0.14);
    const fundamental = Math.sin(TWO_PI * f0 * t + fmIndex * Math.sin(TWO_PI * f0 * modRatio * t));
    const overtone2 = 0.24 * brightness * Math.exp(-t / 0.45) * Math.sin(TWO_PI * f0 * 2 * t);
    const overtone3 = 0.1 * brightness * Math.exp(-t / 0.22) * Math.sin(TWO_PI * f0 * bellRatio * t);

    // Subtle soft felt-hammer transient.
    const clickEnv = Math.exp(-t / 0.004);
    const noise = randSigned() * clickEnv * 0.09 * brightness;
    clickLp += 0.22 * (noise - clickLp);

    const s = (fundamental + overtone2 + overtone3 + clickLp) * env * amp;
    const idx = start + k;
    keysBus.L[idx] += s * gl;
    keysBus.R[idx] += s * gr;
  }
};

// Crystalline modular pluck for rhythmic texture & Generate scramble.
const addArpPluck = (t0, dur, midi, amp, pan = 0, brightness = 1.0) => {
  const start = Math.max(0, Math.floor(t0 * RATE));
  const release = 0.18;
  const end = Math.min(N, Math.floor((t0 + dur + release) * RATE));
  const len = end - start;
  if (len <= 0) return;

  const f0 = hz(midi);
  const [gl, gr] = panGains(pan);
  let lp = 0;

  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    let env = t < 0.0025 ? t / 0.0025 : 1;
    env *= Math.exp(-t / 0.16);
    if (t > dur) env *= Math.max(0, 1 - (t - dur) / release);

    const fm = 0.9 * brightness * Math.exp(-t / 0.055) * Math.sin(TWO_PI * f0 * 2 * t);
    const raw = Math.sin(TWO_PI * f0 * t + fm) + 0.28 * Math.sin(TWO_PI * f0 * 3 * t) * Math.exp(-t / 0.07);
    const cutoff = 500 + 3600 * brightness * Math.exp(-t / 0.08);
    const alpha = clamp((TWO_PI * cutoff) / (TWO_PI * cutoff + RATE), 0.01, 0.7);
    lp += alpha * (raw - lp);

    const s = lp * env * amp;
    const idx = start + k;
    arpBus.L[idx] += s * gl;
    arpBus.R[idx] += s * gr;
  }
};

// Warm round analog sub-bass with filtered pluck envelope.
const addBassNote = (t0, dur, midi, amp, { pluck = 0.65, release = 0.14 } = {}) => {
  const start = Math.max(0, Math.floor(t0 * RATE));
  const end = Math.min(N, Math.floor((t0 + dur + release) * RATE));
  const len = end - start;
  if (len <= 0) return;

  const f0 = hz(midi);
  let lp1 = 0;
  let lp2 = 0;

  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    let env = t < 0.008 ? t / 0.008 : 1;
    env *= 0.78 + 0.22 * Math.exp(-t / 0.22);
    if (t > dur) env *= Math.max(0, 1 - (t - dur) / release);

    const raw =
      Math.sin(TWO_PI * f0 * t) +
      0.34 * Math.sin(TWO_PI * f0 * 2 * t) +
      0.16 * Math.sin(TWO_PI * f0 * 3 * t) +
      0.07 * Math.sin(TWO_PI * f0 * 4 * t);

    const cutoff = 180 + 520 * pluck * Math.exp(-t / 0.11);
    const alpha = clamp((TWO_PI * cutoff) / (TWO_PI * cutoff + RATE), 0.005, 0.45);
    lp1 += alpha * (raw - lp1);
    lp2 += alpha * (lp1 - lp2);

    // Soft analog saturation on bass.
    const s = Math.tanh(lp2 * 1.25) * env * amp;
    const idx = start + k;
    bassBus.L[idx] += s;
    bassBus.R[idx] += s;
  }
};

// Tactile soft sub-kick (warm felt thump, never harsh).
const addKick = (t0, amp = 0.42, duckDepth = 0.24) => {
  registerKickDuck(t0, duckDepth, 0.16);
  const start = Math.max(0, Math.floor(t0 * RATE));
  const len = Math.min(N - start, Math.floor(0.24 * RATE));
  if (len <= 0) return;

  let phase = 0;
  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    const freq = 46 + 74 * Math.exp(-t / 0.024);
    phase += (TWO_PI * freq) / RATE;
    const bodyEnv = (t < 0.002 ? t / 0.002 : 1) * Math.exp(-t / 0.075);
    const click = Math.sin(TWO_PI * 1250 * t) * Math.exp(-t / 0.003) * 0.12;
    const s = Math.tanh((Math.sin(phase) * bodyEnv + click) * 1.15) * amp;
    const idx = start + k;
    percBus.L[idx] += s;
    percBus.R[idx] += s;
  }
};

// Tactile mechanical rim / woody click.
const addRim = (t0, amp = 0.16, pan = 0.15) => {
  const start = Math.max(0, Math.floor(t0 * RATE));
  const len = Math.min(N - start, Math.floor(0.065 * RATE));
  if (len <= 0) return;

  const [gl, gr] = panGains(pan);
  let bp = 0;
  let lp = 0;
  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    const tone =
      (Math.sin(TWO_PI * 820 * t) * 0.55 + Math.sin(TWO_PI * 1680 * t) * 0.45) * Math.exp(-t / 0.009);
    const n = randSigned() * Math.exp(-t / 0.012);
    lp += 0.35 * (n - lp);
    bp += 0.12 * (lp - bp);
    const s = (tone + (lp - bp) * 0.55) * amp;
    const idx = start + k;
    percBus.L[idx] += s * gl;
    percBus.R[idx] += s * gr;
  }
};

// Brushed micro-hat / metallic shaker tick.
const addHat = (t0, amp = 0.075, decay = 0.022, pan = -0.25) => {
  const start = Math.max(0, Math.floor(t0 * RATE));
  const len = Math.min(N - start, Math.floor(Math.max(0.05, decay * 4.5) * RATE));
  if (len <= 0) return;

  const [gl, gr] = panGains(pan);
  let prev1 = 0;
  let prev2 = 0;
  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    const env = (t < 0.0012 ? t / 0.0012 : 1) * Math.exp(-t / decay);
    const n = randSigned();
    // 2nd-order high-pass to keep only silky air above ~6 kHz.
    const hp1 = n - prev1 * 0.86;
    prev1 = n;
    const hp2 = hp1 - prev2 * 0.82;
    prev2 = hp1;
    const shimmer = 0.18 * Math.sin(TWO_PI * 7420 * t) + 0.12 * Math.sin(TWO_PI * 9830 * t);
    const s = (hp2 * 0.45 + shimmer) * env * amp;
    const idx = start + k;
    percBus.L[idx] += s * gl;
    percBus.R[idx] += s * gr;
  }
};

// Soft brushed clap/snare layer for peak energy in Sync & Vault.
const addBrushSnare = (t0, amp = 0.11, pan = 0.06) => {
  const start = Math.max(0, Math.floor(t0 * RATE));
  const len = Math.min(N - start, Math.floor(0.11 * RATE));
  if (len <= 0) return;

  const [gl, gr] = panGains(pan);
  let lp = 0;
  let hpPrev = 0;
  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    const env = (t < 0.003 ? t / 0.003 : 1) * Math.exp(-t / 0.032);
    const n = randSigned();
    const hp = n - hpPrev * 0.72;
    hpPrev = n;
    lp += 0.42 * (hp - lp);
    const body = Math.sin(TWO_PI * 210 * t) * Math.exp(-t / 0.014) * 0.25;
    const s = (lp * 0.65 + body) * env * amp;
    const idx = start + k;
    percBus.L[idx] += s * gl;
    percBus.R[idx] += s * gr;
  }
};

// Subtle filtered air-swell leading into scene transitions.
const addAirSwell = (tEnd, dur = 0.65, amp = 0.055) => {
  const t0 = tEnd - dur;
  const start = Math.max(0, Math.floor(t0 * RATE));
  const end = Math.min(N, Math.floor((tEnd + 0.18) * RATE));
  const len = end - start;
  if (len <= 0) return;

  let lpL = 0;
  let lpR = 0;
  let hpL = 0;
  let hpR = 0;
  for (let k = 0; k < len; k++) {
    const t = k / RATE;
    let env;
    if (t <= dur) {
      const p = t / dur;
      env = p * p * p;
    } else {
      env = Math.exp(-(t - dur) / 0.055);
    }
    const prog = Math.min(1, t / dur);
    const cutoff = 500 + 3400 * prog * prog;
    const alpha = clamp((TWO_PI * cutoff) / (TWO_PI * cutoff + RATE), 0.01, 0.55);

    const nL = randSigned();
    const nR = randSigned();
    const hL = nL - hpL * 0.75;
    hpL = nL;
    const hR = nR - hpR * 0.75;
    hpR = nR;

    lpL += alpha * (hL - lpL);
    lpR += alpha * (hR - lpR);

    const idx = start + k;
    fxBus.L[idx] += lpL * env * amp;
    fxBus.R[idx] += lpR * env * amp;
  }
};

// ============================================================================
// 2. COMPOSITION & ARRANGEMENT (SYNCED TO EXACT SCENE CUTS)
// ============================================================================
// Key: D major (D E F# G A B C#)
// Signature Passpine Motif: F#5 -> E5 -> D5 -> A4 (rising answer B4 -> D5)

// --- SCENE 0: INTRO (0.00 s – 2.50 s) ---------------------------------------
// Warm monochrome breath; crystalline 3-note intro motif tracks logo/title/accent.
addPadChord(0.02, 2.48, [50, 57, 61, 64, 66], 0.17, {
  cutoffStart: 520,
  cutoffEnd: 1150,
  attack: 0.85,
  release: 0.9,
});
addBassNote(0.15, 2.35, 38, 0.15, { pluck: 0.18, release: 0.5 }); // Soft D2 anchor
// Logo (0.07s) -> Title (0.33s) -> Silver Accent (0.60s) -> Description (0.87s)
addKeysNote(0.35, 0.95, 69, 0.16, -0.22, { brightness: 0.85, decay: 1.5 }); // A4
addKeysNote(0.9, 1.0, 78, 0.18, 0.24, { brightness: 0.95, decay: 1.5 }); // F#5
addKeysNote(1.45, 0.95, 76, 0.16, -0.1, { brightness: 0.85, decay: 1.3 }); // E5
addKeysNote(2.0, 0.48, 74, 0.13, 0.15, { brightness: 0.78, decay: 0.8 }); // D5 pickup
addAirSwell(2.5, 0.75, 0.05);

// --- SCENE 1: FILL (2.50 s – 8.50 s, 12 beats = 3 bars) ---------------------
// Cut 1 at 2.50 s: Groove enters cleanly with restrained, tactile electronica pulse.
addPadChord(2.5, 2.0, [50, 57, 61, 64, 66], 0.18, { cutoffStart: 850, cutoffEnd: 1250, attack: 0.25, release: 0.75 }); // Dmaj9
addPadChord(4.5, 2.0, [47, 54, 57, 61, 62], 0.18, { cutoffStart: 900, cutoffEnd: 1300, attack: 0.25, release: 0.75 }); // Bm9
addPadChord(6.5, 1.0, [43, 50, 54, 59, 61], 0.19, { cutoffStart: 950, cutoffEnd: 1350, attack: 0.2, release: 0.65 }); // Gmaj9
addPadChord(7.5, 1.0, [45, 52, 57, 59, 64], 0.19, { cutoffStart: 1050, cutoffEnd: 1550, attack: 0.2, release: 0.7 }); // A6sus4

// Fill Bassline (syncopated warm analog pulse)
const fillBass = [
  [2.5, 0.65, 38],
  [3.25, 0.45, 38],
  [3.75, 0.6, 38],
  [4.5, 0.65, 35],
  [5.25, 0.45, 35],
  [5.75, 0.6, 35],
  [6.5, 0.65, 31],
  [7.25, 0.22, 31],
  [7.5, 0.65, 33],
  [8.25, 0.22, 33],
];
for (const [t, d, m] of fillBass) addBassNote(t, d, m, 0.29, { pluck: 0.65 });

// Fill Main Motif (clear statement, synced to popup open & credential fill)
const fillKeys = [
  [2.5, 0.5, 78, 0.22, -0.25], // F#5
  [3.0, 0.45, 76, 0.19, 0.15], // E5
  [3.5, 0.55, 74, 0.21, -0.15], // D5 (extension popup opens!)
  [4.0, 0.42, 69, 0.16, 0.28], // A4
  [4.5, 0.55, 78, 0.22, -0.22], // F#5
  [5.0, 0.45, 76, 0.19, 0.22], // E5
  [5.5, 0.5, 73, 0.2, -0.1], // C#5
  [6.0, 0.45, 71, 0.17, 0.25], // B4
  [6.5, 0.5, 74, 0.21, -0.2], // D5
  [7.0, 0.4, 78, 0.22, 0.18], // F#5 (Filled badge at 7.17s!)
  [7.25, 0.55, 81, 0.21, 0.3], // A5 crystalline confirm
  [7.75, 0.55, 76, 0.18, -0.18], // E5
];
for (const [t, d, m, a, p] of fillKeys) addKeysNote(t, d, m, a, p, { brightness: 1.0, decay: 1.05 });

// Fill Percussion (soft kick + tactile rim + light brushed hats)
for (let step = 0; step < 48; step++) {
  const t = 2.5 + step * SIXTEENTH;
  const barStep = step % 16;
  if (barStep === 0 || barStep === 10) addKick(t, barStep === 0 ? 0.38 : 0.3, 0.22);
  if (barStep === 4 || barStep === 12) addRim(t, 0.14, barStep === 4 ? 0.18 : -0.18);
  if (barStep % 2 === 0) {
    const accent = barStep % 4 === 2 ? 0.068 : 0.042;
    addHat(t, accent, barStep % 4 === 2 ? 0.026 : 0.016, step % 4 === 0 ? -0.25 : 0.22);
  }
}
addAirSwell(8.5, 0.55, 0.055);

// --- SCENE 2: GENERATE (8.50 s – 14.00 s, 11 beats = 4 + 4 + 3) -------------
// Cut 2 at 8.50 s: Brighter sparkle; 16th-note modular pluck weaves with scramble (10.57–11.63s).
addPadChord(8.5, 2.0, [50, 57, 61, 64, 66], 0.19, { cutoffStart: 1050, cutoffEnd: 1550, attack: 0.2, release: 0.7 }); // Dmaj9
addPadChord(10.5, 2.0, [54, 57, 61, 64, 68], 0.2, { cutoffStart: 1150, cutoffEnd: 1750, attack: 0.2, release: 0.7 }); // F#m9
addPadChord(12.5, 0.75, [43, 50, 54, 59, 61], 0.2, { cutoffStart: 1200, cutoffEnd: 1650, attack: 0.15, release: 0.6 }); // Gmaj9
addPadChord(13.25, 0.75, [45, 52, 57, 59, 64], 0.21, { cutoffStart: 1300, cutoffEnd: 1900, attack: 0.15, release: 0.65 }); // A9sus4

const genBass = [
  [8.5, 0.6, 38],
  [9.25, 0.4, 38],
  [9.75, 0.6, 40], // E2 passing
  [10.5, 0.6, 42], // F#2
  [11.25, 0.4, 42],
  [11.75, 0.6, 42],
  [12.5, 0.6, 31], // G1
  [13.25, 0.65, 33], // A1
];
for (const [t, d, m] of genBass) addBassNote(t, d, m, 0.3, { pluck: 0.75 });

// Generate Motif + "Saved" accent at 12.75 s
const genKeys = [
  [8.5, 0.5, 78, 0.22, -0.2], // F#5
  [9.0, 0.45, 81, 0.21, 0.22], // A5
  [9.5, 0.5, 76, 0.19, -0.15], // E5
  [10.0, 0.45, 74, 0.18, 0.2], // D5
  [10.5, 0.55, 73, 0.2, -0.25], // C#5
  [11.25, 0.5, 76, 0.2, 0.25], // E5
  [11.75, 0.6, 78, 0.22, -0.1], // F#5 (password settles at 11.63s!)
  [12.5, 0.45, 74, 0.2, -0.22], // D5
  [12.75, 0.65, 83, 0.22, 0.24], // B5 ("Saved" toast!)
  [13.5, 0.45, 81, 0.19, 0.05], // A5
];
for (const [t, d, m, a, p] of genKeys) addKeysNote(t, d, m, a, p, { brightness: 1.08, decay: 1.0 });

// Delicate stereo modular pluck arpeggio (intensifies gently during password scramble 10.5–12.5s)
const genArpNotes = [
  [8.5, 10.5, [62, 66, 69, 73, 76, 73, 69, 66], 0.085],
  [10.5, 12.5, [66, 69, 73, 76, 81, 76, 73, 69], 0.115],
  [12.5, 14.0, [67, 71, 74, 78, 81, 76, 73, 69], 0.1],
];
for (const [tStart, tEnd, seq, amp] of genArpNotes) {
  let idx = 0;
  for (let t = tStart; t < tEnd - 0.01; t += SIXTEENTH * 2) {
    const midi = seq[idx % seq.length];
    const pan = ((idx % 4) - 1.5) * 0.32;
    addArpPluck(t, 0.16, midi, amp, pan, 1.05);
    // Subtle 16th ghost sparkle during scramble window
    if (t >= 10.5 && t < 12.25 && idx % 2 === 1) {
      addArpPluck(t + SIXTEENTH, 0.12, midi + 12, amp * 0.52, -pan, 1.2);
    }
    idx++;
  }
}

// Generate Percussion (44 sixteenth steps = 5.50 s)
for (let step = 0; step < 44; step++) {
  const t = 8.5 + step * SIXTEENTH;
  const local = step < 32 ? step % 16 : step - 32; // 16 + 16 + 12
  if (local === 0 || local === 10 || (step === 40)) {
    addKick(t, local === 0 ? 0.4 : 0.31, 0.24);
  }
  if (local === 4 || local === 12 || (step >= 32 && local === 6)) {
    addRim(t, 0.15, local === 4 ? 0.2 : -0.2);
  }
  if (step % 2 === 0 || (step >= 16 && step < 32 && step % 4 === 3)) {
    const isOff = step % 4 === 2;
    addHat(t, isOff ? 0.075 : 0.045, isOff ? 0.028 : 0.015, step % 4 === 0 ? -0.28 : 0.26);
  }
}
addAirSwell(14.0, 0.6, 0.062);

// --- SCENE 3: SYNC (14.00 s – 19.50 s, 11 beats = 4 + 4 + 3) ----------------
// Cut 3 at 14.00 s: Peak momentum & widest stereo bloom. "Synced" hits at 16.00 s!
addPadChord(14.0, 2.0, [47, 54, 57, 61, 66], 0.22, { cutoffStart: 1300, cutoffEnd: 2050, attack: 0.18, release: 0.75 }); // Bm9
addPadChord(16.0, 2.0, [43, 50, 54, 59, 64], 0.23, { cutoffStart: 1450, cutoffEnd: 2200, attack: 0.18, release: 0.75 }); // Gmaj13
addPadChord(18.0, 0.75, [40, 47, 54, 55, 62], 0.21, { cutoffStart: 1350, cutoffEnd: 1850, attack: 0.15, release: 0.65 }); // Em9
addPadChord(18.75, 0.75, [45, 52, 57, 61, 66], 0.22, { cutoffStart: 1450, cutoffEnd: 2100, attack: 0.15, release: 0.7 }); // A13sus4

const syncBass = [
  [14.0, 0.55, 35], // B1
  [14.75, 0.4, 35],
  [15.25, 0.35, 38], // D2
  [15.625, 0.32, 37], // C#2
  [16.0, 0.6, 31], // G1 ("Synced" downbeat!)
  [16.75, 0.45, 31],
  [17.25, 0.6, 35], // B1
  [18.0, 0.6, 40], // E2
  [18.75, 0.65, 33], // A1
];
for (const [t, d, m] of syncBass) addBassNote(t, d, m, 0.32, { pluck: 0.8 });

// Sync Soaring Motif + shimmering bell bloom at 16.00 s ("Synced")
const syncKeys = [
  [14.0, 0.5, 81, 0.23, -0.22], // A5
  [14.5, 0.45, 78, 0.21, 0.2], // F#5
  [15.0, 0.5, 76, 0.2, -0.18], // E5
  [15.5, 0.42, 74, 0.19, 0.16], // D5
  [16.0, 0.85, 86, 0.23, 0.0], // D6 bright "Synced" bell bloom!
  [16.5, 0.5, 81, 0.2, 0.26], // A5 (TOTP card pops in at 16.47s)
  [17.0, 0.5, 78, 0.21, -0.24], // F#5
  [17.5, 0.45, 76, 0.19, 0.2], // E5
  [18.0, 0.5, 74, 0.2, -0.18], // D5
  [18.5, 0.45, 76, 0.19, 0.18], // E5
  [19.0, 0.45, 73, 0.18, -0.12], // C#5
];
for (const [t, d, m, a, p] of syncKeys) addKeysNote(t, d, m, a, p, { brightness: 1.15, decay: 1.15 });

// Sync Driving Subtle Arp
const syncArp = [
  [14.0, 16.0, [62, 66, 71, 73, 78, 73, 71, 66]],
  [16.0, 18.0, [62, 67, 71, 74, 78, 74, 71, 67]],
  [18.0, 19.5, [64, 67, 71, 74, 76, 73, 69, 64]],
];
for (const [tStart, tEnd, seq] of syncArp) {
  let idx = 0;
  for (let t = tStart; t < tEnd - 0.01; t += SIXTEENTH * 2) {
    addArpPluck(t, 0.15, seq[idx % seq.length], 0.095, ((idx % 4) - 1.5) * 0.35, 1.1);
    idx++;
  }
}

// Sync Percussion (fullest groove: soft kick + rim + brushed snare + shimmering hats)
for (let step = 0; step < 44; step++) {
  const t = 14.0 + step * SIXTEENTH;
  const local = step < 32 ? step % 16 : step - 32;
  if (local === 0 || local === 6 || local === 10 || step === 40) {
    addKick(t, local === 0 ? 0.43 : 0.33, 0.27);
  }
  if (local === 4 || local === 12 || (step >= 32 && local === 4)) {
    addRim(t, 0.15, local === 4 ? 0.2 : -0.2);
    addBrushSnare(t, 0.11, local === 4 ? -0.1 : 0.1);
  }
  if (step % 2 === 0 || step % 8 === 7) {
    const isOff = step % 4 === 2;
    addHat(t, isOff ? 0.082 : 0.048, isOff ? 0.032 : 0.016, step % 4 === 0 ? -0.32 : 0.3);
  }
}
addAirSwell(19.5, 0.55, 0.055);

// --- SCENE 4: VAULT (19.50 s – 25.00 s, 11 beats = 4 + 4 + 3) ---------------
// Cut 4 at 19.50 s: Brief 2-beat breath, then grounded groove into the desktop vault reveal (22.77–23.43s).
addPadChord(19.5, 2.0, [42, 50, 57, 61, 64], 0.2, { cutoffStart: 950, cutoffEnd: 1500, attack: 0.25, release: 0.75 }); // Dmaj9/F#
addPadChord(21.5, 2.0, [43, 50, 54, 59, 61], 0.22, { cutoffStart: 1200, cutoffEnd: 1900, attack: 0.2, release: 0.8 }); // Gmaj9
addPadChord(23.5, 1.5, [45, 52, 57, 59, 64], 0.2, { cutoffStart: 1450, cutoffEnd: 1100, attack: 0.2, release: 0.85 }); // A9sus4

const vaultBass = [
  [19.5, 0.9, 42], // F#2 (poised opening)
  [20.5, 0.6, 42], // Import tab clicked!
  [21.125, 0.32, 40], // E2
  [21.5, 0.65, 31], // G1
  [22.25, 0.45, 31],
  [22.75, 0.65, 31], // Vault opens!
  [23.5, 0.75, 33], // A1
  [24.375, 0.55, 33],
];
for (const [t, d, m] of vaultBass) addBassNote(t, d, m, 0.3, { pluck: 0.7 });

// Vault Motif: builds cleanly to desktop reveal (22.77s) and "Imported" toast (23.43s)
const vaultKeys = [
  [19.5, 0.6, 74, 0.2, -0.2], // D5
  [20.25, 0.5, 76, 0.19, 0.18], // E5
  [20.5, 0.55, 78, 0.21, -0.15], // F#5 (Import tab click)
  [21.0, 0.45, 81, 0.2, 0.22], // A5
  [21.5, 0.55, 78, 0.21, -0.22], // F#5
  [22.0, 0.45, 76, 0.19, 0.18], // E5
  [22.5, 0.45, 74, 0.2, -0.15], // D5
  [22.875, 0.65, 81, 0.22, 0.25], // A5 (Desktop reveal)
  [23.5, 0.7, 78, 0.21, -0.18], // F#5 ("Imported" toast settles)
  [24.25, 0.65, 76, 0.18, 0.15], // E5 leading to D5 tonic!
];
for (const [t, d, m, a, p] of vaultKeys) addKeysNote(t, d, m, a, p, { brightness: 1.05, decay: 1.15 });

// Light arp shimmer only during the reveal build (21.5–23.75s)
const vaultArpSeq = [62, 66, 69, 74, 78, 74, 69, 66];
let vIdx = 0;
for (let t = 21.5; t < 23.74; t += SIXTEENTH * 2) {
  addArpPluck(t, 0.16, vaultArpSeq[vIdx % vaultArpSeq.length], 0.085, ((vIdx % 4) - 1.5) * 0.3, 1.0);
  vIdx++;
}

// Vault Percussion: 2-beat breath at 19.5s, re-enters at 20.5s, strips back at 24.0s for Outro
for (let step = 0; step < 38; step++) {
  const t = 19.5 + step * SIXTEENTH;
  if (step < 8) {
    // Minimal hat pulse during the 2-beat breath
    if (step % 4 === 2) addHat(t, 0.045, 0.018, 0.2);
    continue;
  }
  const local = step < 32 ? step % 16 : step - 32;
  if (local === 0 || local === 10 || step === 8) {
    addKick(t, local === 0 || step === 8 ? 0.39 : 0.3, 0.24);
  }
  if (local === 4 || local === 12) {
    addRim(t, 0.14, local === 4 ? 0.18 : -0.18);
    if (step >= 16 && step < 34) addBrushSnare(t, 0.095, -0.08);
  }
  if (step % 2 === 0 && step < 36) {
    const isOff = step % 4 === 2;
    addHat(t, isOff ? 0.07 : 0.04, isOff ? 0.025 : 0.015, step % 4 === 0 ? -0.25 : 0.25);
  }
}
addAirSwell(25.0, 0.7, 0.05);

// --- SCENE 5: OUTRO (25.00 s – 30.00 s, 5.00 s) -----------------------------
// Clean tonic resolution (Dmaj9). Percussion ends with one soft final downbeat thump.
addKick(25.0, 0.3, 0.16);
addPadChord(25.0, 2.7, [50, 57, 61, 64, 66], 0.21, {
  cutoffStart: 1350,
  cutoffEnd: 620,
  attack: 0.25,
  release: 1.85,
});
addBassNote(25.0, 2.55, 38, 0.24, { pluck: 0.28, release: 1.35 }); // Warm D2 tonic anchor

// Unhurried, resolved closing statement of the signature motif -> peaceful Dmaj9 tail
addKeysNote(25.0, 1.4, 74, 0.21, -0.15, { brightness: 0.95, decay: 1.7, release: 0.8 }); // D5 tonic
addKeysNote(25.5, 1.3, 78, 0.19, 0.22, { brightness: 0.92, decay: 1.6, release: 0.8 }); // F#5
addKeysNote(26.0, 1.25, 81, 0.18, -0.2, { brightness: 0.9, decay: 1.6, release: 0.8 }); // A5
addKeysNote(26.625, 1.4, 76, 0.16, 0.18, { brightness: 0.82, decay: 1.7, release: 0.9 }); // E5 (9th)
addKeysNote(27.25, 1.65, 74, 0.19, 0.0, { brightness: 0.78, decay: 1.9, release: 1.0 }); // D5 final resolution
addKeysNote(27.25, 1.65, 69, 0.11, -0.28, { brightness: 0.7, decay: 1.8, release: 1.0 }); // A4 warm fifth
addKeysNote(27.25, 1.65, 86, 0.075, 0.28, { brightness: 0.75, decay: 1.6, release: 0.9 }); // D6 airy octave shimmer

// ============================================================================
// 3. STUDIO EFFECTS: PING-PONG DELAY & DAMPED ALGORITHMIC STEREO REVERB
// ============================================================================

// Dotted-8th / quarter stereo ping-pong delay for keys & arp (warm lowpass feedback).
const applyPingPongDelay = (busL, busR, delaySecL, delaySecR, feedback, wetGain) => {
  const dL = Math.floor(delaySecL * RATE);
  const dR = Math.floor(delaySecR * RATE);
  const tapL = new Float32Array(N);
  const tapR = new Float32Array(N);
  let lpL = 0;
  let lpR = 0;
  const alpha = (TWO_PI * 2800) / (TWO_PI * 2800 + RATE);

  for (let i = 0; i < N; i++) {
    const fbFromR = i >= dL ? tapR[i - dL] * feedback : 0;
    const fbFromL = i >= dR ? tapL[i - dR] * feedback : 0;
    lpL += alpha * (busL[i] + fbFromR - lpL);
    lpR += alpha * (busR[i] + fbFromL - lpR);
    tapL[i] = lpL;
    tapR[i] = lpR;
  }
  for (let i = 0; i < N; i++) {
    if (i >= dL) busL[i] += tapL[i - dL] * wetGain;
    if (i >= dR) busR[i] += tapR[i - dR] * wetGain;
  }
};

applyPingPongDelay(keysBus.L, keysBus.R, 0.375, 0.5, 0.32, 0.24);
applyPingPongDelay(arpBus.L, arpBus.R, 0.25, 0.375, 0.34, 0.28);

// Damped Schroeder/Freeverb-style stereo hall reverb (6 damped combs + 2 allpass diffusers).
const runReverbChannel = (input, combDelays, allpassDelays, feedback, damp) => {
  const wet = new Float32Array(N);
  for (const sec of combDelays) {
    const D = Math.floor(sec * RATE);
    const buf = new Float32Array(N);
    let filt = 0;
    for (let i = 0; i < N; i++) {
      const delayed = i >= D ? buf[i - D] : 0;
      filt = delayed * (1 - damp) + filt * damp;
      buf[i] = input[i] + filt * feedback;
      wet[i] += delayed;
    }
  }
  const scale = 1 / combDelays.length;
  for (let i = 0; i < N; i++) wet[i] *= scale;

  let cur = wet;
  for (const sec of allpassDelays) {
    const D = Math.floor(sec * RATE);
    const next = new Float32Array(N);
    const buf = new Float32Array(N);
    const g = 0.5;
    for (let i = 0; i < N; i++) {
      const delayed = i >= D ? buf[i - D] : 0;
      const x = cur[i];
      buf[i] = x + delayed * g;
      next[i] = delayed - x * g;
    }
    cur = next;
  }
  return cur;
};

// Build reverb send (pads, keys, arp, fx, and just a whisper of percussion — never bass/kick).
const revSendL = new Float32Array(N);
const revSendR = new Float32Array(N);
for (let i = 0; i < N; i++) {
  const sc = sidechain[i];
  revSendL[i] = padBus.L[i] * sc * 0.65 + keysBus.L[i] * 0.55 + arpBus.L[i] * 0.45 + fxBus.L[i] * 0.5 + percBus.L[i] * 0.08;
  revSendR[i] = padBus.R[i] * sc * 0.65 + keysBus.R[i] * 0.55 + arpBus.R[i] * 0.45 + fxBus.R[i] * 0.5 + percBus.R[i] * 0.08;
}

const revL = runReverbChannel(
  revSendL,
  [0.0297, 0.0371, 0.0411, 0.0437, 0.0489, 0.0531],
  [0.0051, 0.0117],
  0.72,
  0.38,
);
const revR = runReverbChannel(
  revSendR,
  [0.0313, 0.0353, 0.0423, 0.0449, 0.0503, 0.0547],
  [0.0057, 0.0123],
  0.72,
  0.38,
);

// ============================================================================
// 4. MIXDOWN & MASTERING (TARGET ~ -15.0 LUFS, TRUE PEAK <= -1.5 dBTP)
// ============================================================================

const mixL = new Float32Array(N);
const mixR = new Float32Array(N);

for (let i = 0; i < N; i++) {
  const sc = sidechain[i];
  const bassSc = 0.3 + 0.7 * sc;
  mixL[i] =
    padBus.L[i] * sc +
    bassBus.L[i] * bassSc +
    keysBus.L[i] +
    arpBus.L[i] +
    percBus.L[i] +
    fxBus.L[i] +
    revL[i] * 0.52;
  mixR[i] =
    padBus.R[i] * sc +
    bassBus.R[i] * bassSc +
    keysBus.R[i] +
    arpBus.R[i] +
    percBus.R[i] +
    fxBus.R[i] +
    revR[i] * 0.52;
}

// Gentle 26 Hz DC/subsonic highpass filter.
let hpInL = 0;
let hpOutL = 0;
let hpInR = 0;
let hpOutR = 0;
const hpCoeff = 1 / (1 + (TWO_PI * 26) / RATE);
for (let i = 0; i < N; i++) {
  const xL = mixL[i];
  const xR = mixR[i];
  hpOutL = hpCoeff * (hpOutL + xL - hpInL);
  hpOutR = hpCoeff * (hpOutR + xR - hpInR);
  hpInL = xL;
  hpInR = xR;
  mixL[i] = hpOutL;
  mixR[i] = hpOutR;
}

// Clean fade-in (0.00–0.05s) and graceful end-of-tail fade to exact silence (28.35–29.85s).
const fadeInEnd = Math.floor(0.05 * RATE);
const fadeOutStart = Math.floor(28.35 * RATE);
const fadeOutEnd = Math.floor(29.85 * RATE);
for (let i = 0; i < N; i++) {
  let env = 1;
  if (i < fadeInEnd) {
    env = 0.5 - 0.5 * Math.cos((Math.PI * i) / fadeInEnd);
  } else if (i >= fadeOutEnd) {
    env = 0;
  } else if (i > fadeOutStart) {
    const p = (i - fadeOutStart) / (fadeOutEnd - fadeOutStart);
    env = 0.5 + 0.5 * Math.cos(Math.PI * p);
  }
  mixL[i] *= env;
  mixR[i] *= env;
}

// Measure active RMS (2.5s–25.0s) and calibrate gain for ~ -15 LUFS with transparent limiter.
let sumSq = 0;
const rmsStart = Math.floor(2.5 * RATE);
const rmsEnd = Math.floor(25.0 * RATE);
for (let i = rmsStart; i < rmsEnd; i++) {
  sumSq += 0.5 * (mixL[i] * mixL[i] + mixR[i] * mixR[i]);
}
const activeRms = Math.sqrt(sumSq / (rmsEnd - rmsStart));
const targetRms = 0.165; // ~ -15.6 dBFS active RMS -> ~ -15.0 to -15.5 LUFS integrated
const makeUpGain = targetRms / Math.max(1e-6, activeRms);

// Smooth lookahead brickwall limiter (ceiling -1.8 dBFS = 0.8128 so AAC true-peak stays <= -1.5 dBTP).
const ceiling = 0.8128;
const lookahead = Math.floor(0.005 * RATE);
const gainEnv = new Float32Array(N).fill(1);
let curGain = 1;
const attackCoeff = Math.exp(-1 / (0.002 * RATE));
const releaseCoeff = Math.exp(-1 / (0.085 * RATE));

for (let i = 0; i < N; i++) {
  const ahead = Math.min(N - 1, i + lookahead);
  const mag = Math.max(Math.abs(mixL[ahead]), Math.abs(mixR[ahead])) * makeUpGain;
  const desired = mag > ceiling ? ceiling / mag : 1;
  if (desired < curGain) {
    curGain = attackCoeff * curGain + (1 - attackCoeff) * desired;
  } else {
    curGain = releaseCoeff * curGain + (1 - releaseCoeff) * desired;
  }
  gainEnv[i] = curGain;
}

let finalPeak = 0;
for (let i = 0; i < N; i++) {
  const g = makeUpGain * gainEnv[i];
  mixL[i] = clamp(mixL[i] * g, -ceiling, ceiling);
  mixR[i] = clamp(mixR[i] * g, -ceiling, ceiling);
  finalPeak = Math.max(finalPeak, Math.abs(mixL[i]), Math.abs(mixR[i]));
}

// Write 16-bit stereo 48 kHz WAV.
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
  out.writeInt16LE(Math.round(mixL[i] * 32767), 44 + i * 4);
  out.writeInt16LE(Math.round(mixR[i] * 32767), 46 + i * 4);
}
writeFileSync(new URL("../public/music.wav", import.meta.url), out);

const peakDb = 20 * Math.log10(Math.max(1e-9, finalPeak));
console.log(
  `wrote public/music.wav (${SECONDS}s @ ${RATE}Hz stereo), makeup ${makeUpGain.toFixed(2)}x, peak ${peakDb.toFixed(2)} dBFS`,
);
