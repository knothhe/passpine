import React from "react";
import type { ReactNode } from "react";
import { useCurrentFrame } from "remotion";
import { FeatureCard, Toast, Window, typed } from "../components";
import { copy } from "../copy";
import { fonts, ink, muted, pop, ramp, sceneFade, timing } from "../theme";

const c = copy.generate;
const pool = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%&*";

/** Deterministic scramble that settles on the final password. */
const scramble = (frame: number, settleAt: number) =>
  frame >= settleAt
    ? c.password
    : Array.from(c.password, (_, i) => pool[(i * 7 + Math.floor(frame / 2) * 13 + i * i) % pool.length]).join("");

const Field = ({ label, active, children }: { label: string; active: boolean; children: ReactNode }) => (
  <div style={{ marginBottom: 24 }}>
    <div style={{ fontSize: 17, color: muted, marginBottom: 10 }}>{label}</div>
    <div
      style={{
        height: 56,
        borderRadius: 12,
        border: `2px solid ${active ? ink : "#e2e2df"}`,
        display: "flex",
        alignItems: "center",
        padding: "0 18px",
        fontSize: 21,
      }}
    >
      {children}
    </div>
  </div>
);

export const Generate = () => {
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 26);
  const name = typed(c.name, frame, 16, 32);
  const username = typed(c.username, frame, 34, 52);
  const generatorIn = pop(frame, 58);
  const generatorOut = ramp(frame, 106, 116);
  const generator = Math.min(generatorIn, 1 - generatorOut);
  const filledIn = frame >= 106;
  const savePressed = frame >= 124 && frame < 129;
  const saved = Math.min(pop(frame, 130), 1);

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.generate.duration) }}>
      <FeatureCard index={2} title={c.title} caption={c.caption} />
      <div style={{ position: "absolute", left: 800, top: 190, opacity: enter, transform: `translateY(${(1 - enter) * 40}px)` }}>
        <Window width={1000} height={640} title={c.window}>
          <div style={{ position: "absolute", left: 48, top: 40, width: 500 }}>
            <Field label={c.nameLabel} active={frame >= 16 && frame < 34}>
              {name}
            </Field>
            <Field label={c.usernameLabel} active={frame >= 34 && frame < 54}>
              {username}
            </Field>
            <Field label={c.passwordLabel} active={frame >= 54 && frame < 120}>
              {filledIn ? (
                <span style={{ fontFamily: fonts.mono, fontSize: 20, letterSpacing: 1 }}>{c.password}</span>
              ) : (
                <span style={{ color: "#9a9a96" }}>{c.passwordPlaceholder}</span>
              )}
              <span
                style={{
                  marginLeft: "auto",
                  fontSize: 16,
                  padding: "6px 12px",
                  borderRadius: 8,
                  background: frame >= 52 && frame < 58 ? "#dcdcd9" : "#efefed",
                  whiteSpace: "nowrap",
                }}
              >
                {c.generateButton}
              </span>
            </Field>
            <div
              style={{
                marginTop: 18,
                width: 180,
                height: 54,
                borderRadius: 12,
                background: ink,
                color: "#fff",
                display: "grid",
                placeItems: "center",
                fontSize: 20,
                fontWeight: 600,
                transform: `scale(${savePressed ? 0.96 : 1})`,
              }}
            >
              {c.save}
            </div>
          </div>

          {/* Generator popover */}
          <div
            style={{
              position: "absolute",
              left: 580,
              top: 120,
              width: 380,
              padding: "24px 26px",
              borderRadius: 16,
              background: "#fff",
              boxShadow: "0 24px 60px rgba(0,0,0,0.16), 0 0 0 1px rgba(0,0,0,0.07)",
              opacity: generator,
              transform: `translateX(${(1 - generatorIn) * -20}px)`,
            }}
          >
            <div style={{ fontSize: 20, fontWeight: 700 }}>{c.generator}</div>
            <div
              style={{
                marginTop: 18,
                padding: "16px 14px",
                borderRadius: 10,
                background: "#f4f4f2",
                fontFamily: fonts.mono,
                fontSize: 21,
                letterSpacing: 1,
                textAlign: "center",
              }}
            >
              {scramble(frame, 92)}
            </div>
            <div style={{ display: "flex", alignItems: "center", marginTop: 20, fontSize: 18 }}>
              <span style={{ width: 60, color: muted }}>{c.length}</span>
              <div style={{ flex: 1, height: 4, borderRadius: 2, background: "#e2e2df", position: "relative" }}>
                <div style={{ width: "40%", height: "100%", borderRadius: 2, background: ink }} />
                <div style={{ position: "absolute", left: "40%", top: -8, width: 20, height: 20, borderRadius: 10, background: "#fff", border: `2px solid ${ink}`, transform: "translateX(-50%)" }} />
              </div>
              <span style={{ width: 40, textAlign: "right", fontWeight: 600 }}>{c.lengthValue}</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", marginTop: 18, fontSize: 18 }}>
              <span style={{ color: muted }}>{c.symbols}</span>
              <div style={{ marginLeft: "auto", width: 44, height: 26, borderRadius: 13, background: ink, position: "relative" }}>
                <div style={{ position: "absolute", right: 3, top: 3, width: 20, height: 20, borderRadius: 10, background: "#fff" }} />
              </div>
            </div>
            <div
              style={{
                marginTop: 22,
                height: 50,
                borderRadius: 10,
                background: ink,
                color: "#fff",
                display: "grid",
                placeItems: "center",
                fontSize: 18,
                fontWeight: 600,
                transform: `scale(${frame >= 100 && frame < 105 ? 0.96 : 1})`,
              }}
            >
              {c.fillButton}
            </div>
          </div>
          <Toast text={c.saved} progress={saved} />
        </Window>
      </div>
    </div>
  );
};
