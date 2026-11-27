import React from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { Check, Cursor, FeatureCard, LockIcon, Window, typed } from "../components";
import { useCopy } from "../copy";
import { fonts, palette, pop, ramp, sceneFade, timing } from "../theme";

const Field = ({ label, value, active }: { label: string; value: string; active: boolean }) => (
  <div style={{ marginTop: 22 }}>
    <div style={{ fontSize: 16, color: palette.inkMuted, marginBottom: 8, fontWeight: 500 }}>{label}</div>
    <div
      style={{
        height: 56,
        borderRadius: 12,
        border: `1.5px solid ${active ? palette.ink : palette.borderStrong}`,
        boxShadow: active ? `0 0 0 3px ${palette.silverSoft}` : "none",
        display: "flex",
        alignItems: "center",
        padding: "0 18px",
        fontSize: 21,
        color: palette.ink,
        background: value ? palette.panelElevated : palette.graphite,
        fontFamily: fonts.sans,
      }}
    >
      {value}
    </div>
  </div>
);

export const Fill = () => {
  const copy = useCopy();
  const c = copy.fill;
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 24);

  const popupIn = pop(frame, 30);
  const popupOut = ramp(frame, 92, 104);
  const popupOpacity = Math.max(0, Math.min(popupIn, 1 - popupOut));
  const rowHighlight = ramp(frame, 64, 74);

  const username = typed(c.username, frame, 98, 120);
  const dots = "•".repeat(
    Math.round(interpolate(frame, [118, 132], [0, 14], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })),
  );
  const pressed = frame >= 136 && frame < 142;
  const filled = pop(frame, 140);

  // Cursor path: centre -> extension button -> first real row in extension.png -> away.
  const cx = interpolate(frame, [14, 28, 52, 68, 96, 114], [480, 996, 996, 800, 800, 1090], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const cy = interpolate(frame, [14, 28, 52, 68, 96, 114], [360, -26, -26, 134, 134, 620], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const cursorOpacity = interpolate(frame, [10, 16, 100, 112], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const clicking = (frame >= 28 && frame < 34) || (frame >= 82 && frame < 88);

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.fill.duration) }}>
      <FeatureCard index={1} eyebrow={c.eyebrow} title={c.title} caption={c.caption} spec={c.spec} />

      <div
        style={{
          position: "absolute",
          left: 752,
          top: 156,
          opacity: enter,
          transform: `translateY(${(1 - enter) * 36}px)`,
        }}
      >
        <Window
          width={1056}
          height={680}
          title={
            <div
              style={{
                width: 480,
                height: 30,
                borderRadius: 15,
                background: palette.graphite,
                border: `1px solid ${palette.border}`,
                display: "inline-flex",
                alignItems: "center",
                gap: 8,
                padding: "0 14px",
                fontSize: 14,
                fontFamily: fonts.mono,
                color: palette.silver,
              }}
            >
              <LockIcon size={13} color={palette.inkMuted} />
              <span>{c.site}</span>
            </div>
          }
          toolbar={
            <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
              <div
                style={{
                  width: 34,
                  height: 34,
                  borderRadius: 9,
                  background: popupIn > 0.05 && popupOut < 1 ? palette.panelElevated : "transparent",
                  border: `1px solid ${popupIn > 0.05 && popupOut < 1 ? palette.silverBorder : "transparent"}`,
                  display: "grid",
                  placeItems: "center",
                }}
              >
                <Img src={staticFile("icon.svg")} width={24} height={24} />
              </div>
            </div>
          }
        >
          {/* Dark monochrome login surface */}
          <div
            style={{
              position: "absolute",
              inset: 0,
              background: `radial-gradient(640px 420px at 32% 28%, rgba(196, 196, 204, 0.05) 0%, transparent 70%), ${palette.graphite}`,
            }}
          >
            <div
              style={{
                position: "absolute",
                left: 112,
                top: 96,
                width: 448,
                padding: "36px 38px",
                borderRadius: 18,
                background: palette.panel,
                border: `1px solid ${palette.borderStrong}`,
                boxShadow: "0 24px 60px rgba(0, 0, 0, 0.35)",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <div style={{ fontSize: 30, fontWeight: 700, color: palette.ink, letterSpacing: -0.6 }}>
                  {c.loginTitle}
                </div>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 6,
                    padding: "6px 14px",
                    borderRadius: 999,
                    background: palette.ink,
                    color: palette.graphite,
                    fontSize: 15,
                    fontWeight: 700,
                    opacity: filled,
                    transform: `scale(${0.76 + 0.24 * filled})`,
                  }}
                >
                  <Check size={16} color={palette.graphite} />
                  <span>{c.filled}</span>
                </div>
              </div>

              <Field label={c.emailLabel} value={username} active={frame >= 98 && frame < 118} />
              <Field label={c.passwordLabel} value={dots} active={frame >= 118 && frame < 134} />

              <div
                style={{
                  marginTop: 30,
                  height: 54,
                  borderRadius: 12,
                  background: palette.ink,
                  color: palette.graphite,
                  display: "grid",
                  placeItems: "center",
                  fontSize: 19,
                  fontWeight: 700,
                  transform: `scale(${pressed ? 0.97 : 1})`,
                }}
              >
                {c.signIn}
              </div>
            </div>
          </div>

          {/* REAL Passpine Browser Extension UI screenshot (full original colors preserved) */}
          <div
            style={{
              position: "absolute",
              right: 22,
              top: 12,
              width: 392,
              borderRadius: 16,
              overflow: "hidden",
              background: "#ffffff",
              border: `1px solid ${palette.silverBorder}`,
              boxShadow: `0 28px 74px rgba(0, 0, 0, 0.62), 0 0 0 1px rgba(237, 237, 241, 0.14)`,
              opacity: popupOpacity,
              transform: `translateY(${(1 - popupIn) * -14}px) scale(${0.95 + 0.05 * popupIn})`,
              transformOrigin: "top right",
            }}
          >
            <div style={{ position: "relative", width: "100%" }}>
              <Img
                src={staticFile(`demo/${copy.locale}/extension.png`)}
                style={{ width: "100%", height: "auto", display: "block" }}
              />
              {/* Subtle monochrome selection highlight over the first real github.com entry */}
              <div
                style={{
                  position: "absolute",
                  left: 10,
                  right: 10,
                  top: 86,
                  height: 62,
                  borderRadius: 10,
                  border: `2px solid ${palette.graphite}`,
                  background: "rgba(28, 28, 33, 0.08)",
                  boxShadow: "0 4px 16px rgba(28, 28, 33, 0.14)",
                  opacity: rowHighlight,
                  pointerEvents: "none",
                }}
              />
            </div>
          </div>

          <div style={{ opacity: cursorOpacity }}>
            <Cursor x={cx} y={cy} pressed={clicking} />
          </div>
        </Window>
      </div>
    </div>
  );
};
