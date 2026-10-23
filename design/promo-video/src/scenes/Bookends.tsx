import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { Logo } from "../components";
import { useCopy } from "../copy";
import { fonts, ink, muted, pop, ramp } from "../theme";

const center = { position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", color: ink, fontFamily: fonts.sans } as const;

const Lockup = ({ size, progress }: { size: number; progress: number }) => {
  const copy = useCopy();
  return (
    <div style={{ display: "flex", alignItems: "center", gap: size * 0.2, opacity: progress, transform: `scale(${0.9 + 0.1 * progress})` }}>
      <Logo size={size} />
      <div>
        <div style={{ fontSize: size * 0.56, fontWeight: 700, letterSpacing: -1, lineHeight: 1.1 }}>{copy.brand}</div>
        <div style={{ fontSize: size * 0.2, color: muted, marginTop: 6 }}>{copy.description}</div>
      </div>
    </div>
  );
};

export const Intro = () => {
  const frame = useCurrentFrame();
  const opacity = interpolate(frame, [44, 60], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div style={{ ...center, opacity }}>
      <Lockup size={130} progress={pop(frame, 2)} />
    </div>
  );
};

export const Outro = () => {
  const copy = useCopy();
  const frame = useCurrentFrame();
  const line = (delay: number) => {
    const t = ramp(frame, delay, delay + 22);
    return { opacity: t, transform: `translateY(${(1 - t) * 16}px)` };
  };
  const fadeOut = interpolate(frame, [134, 150], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div style={{ ...center, opacity: fadeOut }}>
      <Lockup size={150} progress={pop(frame, 4)} />
      <div style={{ fontSize: 28, marginTop: 72, letterSpacing: 2, color: "#4a4a47", ...line(24) }}>{copy.outro.points}</div>
      <div style={{ fontSize: 24, marginTop: 18, letterSpacing: 4, color: muted, ...line(34) }}>{copy.outro.platforms}</div>
    </div>
  );
};
