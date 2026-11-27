import { Easing, interpolate, spring } from "remotion";

export const FPS = 30;

// Scene boundaries in frames (30 fps, 30.0 s total = 900 frames).
export const timing = {
  intro: { from: 0, duration: 75 },
  fill: { from: 75, duration: 180 },
  generate: { from: 255, duration: 165 },
  sync: { from: 420, duration: 165 },
  vault: { from: 585, duration: 165 },
  outro: { from: 750, duration: 150 },
  total: 900,
} as const;

export const features = [timing.fill, timing.generate, timing.sync, timing.vault] as const;

export const fonts = {
  sans: '"-apple-system", "SF Pro Display", "Inter", "PingFang SC", "Hiragino Sans GB", "Noto Sans CJK SC", system-ui, sans-serif',
  mono: '"SF Mono", "Maple Mono NF CN", "Menlo", "JetBrains Mono", "Noto Sans Mono CJK SC", ui-monospace, monospace',
} as const;

// Passpine default Monochrome palette: graphite, layered slate panels, silver accents, bright & muted ink.
export const palette = {
  graphite: "#1c1c21",
  graphiteDeep: "#141418",
  panel: "#25262c",
  panelElevated: "#2b2c30",
  panelGlass: "rgba(37, 38, 44, 0.92)",
  silver: "#c4c4cc",
  silverSoft: "rgba(196, 196, 204, 0.12)",
  silverGlow: "rgba(196, 196, 204, 0.24)",
  silverBorder: "rgba(196, 196, 204, 0.34)",
  ink: "#ededf1",
  inkSecondary: "#c4c4cc",
  inkMuted: "#a1a1aa",
  inkSubtle: "#71717a",
  border: "rgba(237, 237, 241, 0.10)",
  borderStrong: "rgba(237, 237, 241, 0.20)",
  windowChrome: "#25262c",
  windowChromeTop: "#2b2c30",
  windowChromeBorder: "rgba(237, 237, 241, 0.09)",
} as const;

const easeOutExpo = Easing.bezier(0.22, 1, 0.36, 1);

/** 0->1 ramp between two local frames with a refined ease-out curve. */
export const ramp = (frame: number, start: number, end: number) =>
  interpolate(frame, [start, end], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: easeOutExpo,
  });

export const pop = (frame: number, delay: number) =>
  spring({
    frame: frame - delay,
    fps: FPS,
    config: { damping: 18, stiffness: 150, mass: 0.85 },
  });

/** Smooth fade in at the start and fade out at the end of a scene. */
export const sceneFade = (frame: number, duration: number, edge = 12) =>
  interpolate(frame, [0, edge, duration - edge, duration], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
