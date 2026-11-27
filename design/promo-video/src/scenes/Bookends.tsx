import React from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { Check, Logo } from "../components";
import { useCopy } from "../copy";
import { fonts, palette, pop, ramp, timing } from "../theme";

export const Intro = () => {
  const c = useCopy();
  const frame = useCurrentFrame();
  const fadeOut = interpolate(frame, [timing.intro.duration - 16, timing.intro.duration], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const logoIn = pop(frame, 2);
  const eyebrowIn = ramp(frame, 6, 22);
  const titleIn = ramp(frame, 10, 28);
  const accentIn = ramp(frame, 18, 36);
  const descIn = ramp(frame, 26, 44);
  const zoom = interpolate(frame, [0, timing.intro.duration], [0.97, 1.02], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        fontFamily: fonts.sans,
        opacity: fadeOut,
        transform: `scale(${zoom})`,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 18,
          opacity: logoIn,
          transform: `scale(${0.9 + 0.1 * logoIn})`,
        }}
      >
        <Logo size={88} />
        <span
          style={{
            fontSize: 48,
            fontWeight: 700,
            color: palette.ink,
            letterSpacing: -1.4,
          }}
        >
          {c.brand}
        </span>
      </div>

      <div
        style={{
          marginTop: 42,
          padding: "7px 16px",
          borderRadius: 999,
          background: palette.panel,
          border: `1px solid ${palette.borderStrong}`,
          fontFamily: fonts.mono,
          fontSize: 15,
          letterSpacing: 2.8,
          color: palette.silver,
          opacity: eyebrowIn,
          transform: `translateY(${(1 - eyebrowIn) * 14}px)`,
        }}
      >
        {c.eyebrow}
      </div>

      <div
        style={{
          marginTop: 28,
          display: "flex",
          flexWrap: "wrap",
          justifyContent: "center",
          gap: c.locale === "zh-CN" ? 6 : 24,
          fontSize: 102,
          fontWeight: 700,
          letterSpacing: c.locale === "zh-CN" ? -2 : -3.5,
          lineHeight: 1.06,
        }}
      >
        <span
          style={{
            color: palette.ink,
            opacity: titleIn,
            transform: `translateY(${(1 - titleIn) * 26}px)`,
          }}
        >
          {c.heroTitle}
        </span>
        <span
          style={{
            color: palette.silver,
            opacity: accentIn,
            transform: `translateY(${(1 - accentIn) * 26}px)`,
          }}
        >
          {c.heroAccent}
        </span>
      </div>

      <div
        style={{
          marginTop: 28,
          maxWidth: 880,
          textAlign: "center",
          fontSize: 28,
          lineHeight: 1.55,
          color: palette.inkMuted,
          opacity: descIn,
          transform: `translateY(${(1 - descIn) * 16}px)`,
        }}
      >
        {c.description}
      </div>
    </div>
  );
};

export const Outro = () => {
  const c = useCopy();
  const frame = useCurrentFrame();
  const fadeIn = ramp(frame, 0, 18);
  const fadeOut = interpolate(frame, [timing.outro.duration - 16, timing.outro.duration], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const line = (delay: number) => {
    const t = ramp(frame, delay, delay + 22);
    return { opacity: t, transform: `translateY(${(1 - t) * 18}px)` };
  };
  const drift = interpolate(frame, [0, timing.outro.duration], [0, 16], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        fontFamily: fonts.sans,
        opacity: Math.min(fadeIn, fadeOut),
      }}
    >
      {/* Subtle receding real UI silhouettes framing the outro */}
      <div
        style={{
          position: "absolute",
          left: -120,
          top: 170,
          width: 560,
          borderRadius: 16,
          overflow: "hidden",
          opacity: 0.14 * fadeIn,
          transform: `rotate(-4deg) translateY(${-drift}px)`,
          border: `1px solid ${palette.borderStrong}`,
          boxShadow: "0 30px 80px rgba(0, 0, 0, 0.55)",
        }}
      >
        <Img src={staticFile(`demo/${c.locale}/desktop.png`)} style={{ width: "100%", height: "auto", display: "block" }} />
      </div>
      <div
        style={{
          position: "absolute",
          right: -60,
          bottom: 130,
          width: 310,
          borderRadius: 16,
          overflow: "hidden",
          opacity: 0.16 * fadeIn,
          transform: `rotate(4deg) translateY(${drift}px)`,
          border: `1px solid ${palette.borderStrong}`,
          boxShadow: "0 30px 80px rgba(0, 0, 0, 0.55)",
        }}
      >
        <Img src={staticFile(`demo/${c.locale}/extension.png`)} style={{ width: "100%", height: "auto", display: "block" }} />
      </div>

      {/* Centerpiece brand lockup */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 24,
          opacity: pop(frame, 4),
          transform: `scale(${0.92 + 0.08 * pop(frame, 4)})`,
        }}
      >
        <Logo size={118} />
        <div>
          <div style={{ fontSize: 66, fontWeight: 700, color: palette.ink, letterSpacing: -2, lineHeight: 1.05 }}>
            {c.brand}
          </div>
          <div style={{ fontSize: 28, color: palette.silver, marginTop: 8, fontWeight: 500 }}>
            {c.heroTitle} {c.heroAccent}
          </div>
        </div>
      </div>

      {/* Three trust pills */}
      <div
        style={{
          marginTop: 58,
          display: "flex",
          alignItems: "center",
          gap: 18,
          ...line(22),
        }}
      >
        {c.outro.points.map((point) => (
          <div
            key={point}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 10,
              padding: "12px 22px",
              borderRadius: 999,
              background: palette.panel,
              border: `1px solid ${palette.borderStrong}`,
              fontSize: 21,
              color: palette.ink,
              fontWeight: 500,
            }}
          >
            <Check size={18} color={palette.silver} />
            <span>{point}</span>
          </div>
        ))}
      </div>

      <div
        style={{
          marginTop: 34,
          fontSize: 22,
          letterSpacing: 4,
          color: palette.inkMuted,
          fontFamily: fonts.mono,
          ...line(34),
        }}
      >
        {c.outro.platforms}
      </div>

      <div
        style={{
          marginTop: 14,
          fontSize: 18,
          letterSpacing: 1.5,
          color: palette.inkSubtle,
          fontFamily: fonts.mono,
          ...line(42),
        }}
      >
        {c.outro.url}
      </div>
    </div>
  );
};
