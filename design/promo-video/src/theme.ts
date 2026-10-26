import { Easing, interpolate, spring } from "remotion";

export const FPS = 30;

// Scene boundaries in frames (30 fps, 30 s total).
export const timing = {
  intro: { from: 0, duration: 60 },
  fill: { from: 60, duration: 180 },
  generate: { from: 240, duration: 165 },
  sync: { from: 405, duration: 165 },
  vault: { from: 570, duration: 180 },
  outro: { from: 750, duration: 150 },
  total: 900,
};

export const features = [timing.fill, timing.generate, timing.sync, timing.vault];

export const fonts = {
  sans: '"Inter", "Noto Sans CJK SC", system-ui, sans-serif',
  mono: '"Maple Mono NF CN", "Noto Sans Mono CJK SC", monospace',
};

export const ink = "#171918";
export const paper = "#f5f5f3";
export const muted = "#7a7a76";

const ease = Easing.bezier(0.22, 1, 0.36, 1);

/** 0→1 ramp between two local frames with a soft ease-out. */
export const ramp = (frame: number, start: number, end: number) =>
  interpolate(frame, [start, end], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: ease,
  });

export const pop = (frame: number, delay: number) =>
  spring({ frame: frame - delay, fps: FPS, config: { damping: 18, stiffness: 140 } });

/** Fade in at the start and out at the end of a scene. */
export const sceneFade = (frame: number, duration: number, edge = 12) =>
  interpolate(frame, [0, edge, duration - edge, duration], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
