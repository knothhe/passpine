import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { Check, Cursor, FeatureCard, Logo, Window, typed } from "../components";
import { useCopy } from "../copy";
import { ink, pop, ramp, sceneFade, timing } from "../theme";


const Field = ({ label, value, active }: { label: string; value: string; active: boolean }) => (
  <div style={{ marginTop: 26 }}>
    <div style={{ fontSize: 18, color: "#6b6b68", marginBottom: 10 }}>{label}</div>
    <div
      style={{
        height: 58,
        borderRadius: 12,
        border: `2px solid ${active ? ink : "#e2e2df"}`,
        display: "flex",
        alignItems: "center",
        padding: "0 18px",
        fontSize: 22,
        background: value ? "#f6f8ff" : "#fff",
      }}
    >
      {value}
    </div>
  </div>
);

export const Fill = () => {
  const c = useCopy().fill;
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 26);

  const popupIn = pop(frame, 32);
  const popupOut = ramp(frame, 78, 90);
  const popupOpacity = Math.min(popupIn, 1 - popupOut);
  const rowHighlight = ramp(frame, 58, 66);

  const username = typed(c.username, frame, 86, 110);
  const dots = "•".repeat(Math.round(interpolate(frame, [108, 122], [0, 12], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })));
  const pressed = frame >= 126 && frame < 132;
  const filled = pop(frame, 132);

  // Cursor path: centre → extension icon → first entry → away.
  const cx = interpolate(frame, [18, 32, 48, 62, 92, 110], [520, 952, 952, 760, 760, 1100], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const cy = interpolate(frame, [18, 32, 48, 62, 92, 110], [380, -36, -36, 170, 170, 700], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const cursorOpacity = interpolate(frame, [14, 20, 96, 108], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const clicking = (frame >= 32 && frame < 37) || (frame >= 70 && frame < 76);

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.fill.duration) }}>
      <FeatureCard index={1} title={c.title} caption={c.caption} />
      <div style={{ position: "absolute", left: 800, top: 190, opacity: enter, transform: `translateY(${(1 - enter) * 40}px)` }}>
        <Window
          width={1000}
          height={640}
          toolbar={
            <div style={{ width: 34, height: 34, borderRadius: 8, background: popupIn > 0.05 && popupOut < 1 ? "#e3e3e0" : "transparent", display: "grid", placeItems: "center" }}>
              <Logo size={30} />
            </div>
          }
        >
          <div style={{ position: "absolute", left: 150, top: -44, width: 520, height: 32, borderRadius: 16, background: "#fff", border: "1px solid #e2e2df", display: "flex", alignItems: "center", padding: "0 16px", fontSize: 17, color: "#555" }}>
            {c.site}
          </div>
          <div style={{ position: "absolute", left: 290, top: 70, width: 420 }}>
            <div style={{ fontSize: 34, fontWeight: 700 }}>{c.loginTitle}</div>
            <Field label={c.emailLabel} value={username} active={frame >= 86 && frame < 108} />
            <Field label={c.passwordLabel} value={dots} active={frame >= 108 && frame < 124} />
            <div
              style={{
                marginTop: 34,
                height: 60,
                borderRadius: 12,
                background: ink,
                color: "#fff",
                display: "grid",
                placeItems: "center",
                fontSize: 22,
                fontWeight: 600,
                transform: `scale(${pressed ? 0.97 : 1})`,
              }}
            >
              {c.signIn}
            </div>
            <div
              style={{
                position: "absolute",
                right: 0,
                top: 0,
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "8px 16px",
                borderRadius: 20,
                background: "#e8f6ee",
                color: "#1f7a46",
                fontSize: 19,
                fontWeight: 600,
                opacity: filled,
                transform: `scale(${0.7 + 0.3 * filled})`,
              }}
            >
              <Check size={20} color="#1f7a46" /> {c.filled}
            </div>
          </div>

          {/* Extension popup */}
          <div
            style={{
              position: "absolute",
              right: 18,
              top: 8,
              width: 380,
              borderRadius: 14,
              background: "#f4f4f2",
              boxShadow: "0 24px 60px rgba(0,0,0,0.18), 0 0 0 1px rgba(0,0,0,0.06)",
              opacity: popupOpacity,
              transform: `translateY(${(1 - popupIn) * -16}px) scale(${0.96 + 0.04 * popupIn})`,
              transformOrigin: "top right",
              overflow: "hidden",
            }}
          >
            <div style={{ padding: "18px 20px 10px", fontSize: 19, fontWeight: 600 }}>{c.site}</div>
            <div style={{ margin: "0 16px", height: 42, borderRadius: 10, background: "#fff", display: "flex", alignItems: "center", padding: "0 14px", color: "#9a9a96", fontSize: 17 }}>
              {c.popupSearch}
            </div>
            <div style={{ padding: "14px 20px 6px", fontSize: 15, color: "#7a7a76" }}>{c.popupSection}</div>
            {c.entries.map((e, i) => (
              <div
                key={e.name}
                style={{
                  margin: "0 8px",
                  padding: "10px 12px",
                  borderRadius: 10,
                  background: i === 0 ? `rgba(23,25,24,${0.08 * rowHighlight})` : "transparent",
                }}
              >
                <div style={{ fontSize: 14, color: "#7a7a76" }}>{e.group}</div>
                <div style={{ fontSize: 19, fontWeight: 600 }}>{e.name}</div>
              </div>
            ))}
            <div style={{ marginTop: 10, padding: "12px 20px", borderTop: "1px solid #e2e2df", fontSize: 15, color: "#555", display: "flex", alignItems: "center", gap: 8 }}>
              <div style={{ width: 9, height: 9, borderRadius: 5, background: "#22a35a" }} />
              {c.popupStatus}
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
