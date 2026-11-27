import React from "react";
import type { ReactNode } from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { FeatureCard, Toast, Window, typed } from "../components";
import { useCopy } from "../copy";
import { fonts, palette, pop, ramp, sceneFade, timing } from "../theme";

const pool = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%&*";

/** Deterministic scramble that settles cleanly on the final strong password. */
const scramble = (password: string, frame: number, settleAt: number) =>
  frame >= settleAt
    ? password
    : Array.from(password, (_, i) => pool[(i * 7 + Math.floor(frame / 2) * 13 + i * i) % pool.length]).join("");

const Field = ({ label, active, children }: { label: string; active: boolean; children: ReactNode }) => (
  <div style={{ marginBottom: 18 }}>
    <div style={{ fontSize: 15, color: palette.inkMuted, marginBottom: 7, fontWeight: 500 }}>{label}</div>
    <div
      style={{
        height: 48,
        borderRadius: 10,
        border: `1.5px solid ${active ? palette.ink : palette.borderStrong}`,
        background: palette.graphite,
        display: "flex",
        alignItems: "center",
        padding: "0 14px",
        fontSize: 18,
        color: palette.ink,
      }}
    >
      {children}
    </div>
  </div>
);

export const Generate = () => {
  const copy = useCopy();
  const c = copy.generate;
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 24);
  const bgScale = interpolate(frame, [0, timing.generate.duration], [1.0, 1.03], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const modalIn = pop(frame, 14);
  const name = typed(c.name, frame, 24, 40);
  const username = typed(c.username, frame, 42, 60);
  const genPanel = ramp(frame, 62, 74);
  const filledIn = frame >= 104;
  const savePressed = frame >= 122 && frame < 128;
  const saved = Math.min(pop(frame, 128), 1);

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.generate.duration) }}>
      <FeatureCard index={2} eyebrow={c.eyebrow} title={c.title} caption={c.caption} spec={c.spec} />

      <div
        style={{
          position: "absolute",
          left: 752,
          top: 156,
          opacity: enter,
          transform: `translateY(${(1 - enter) * 36}px)`,
        }}
      >
        <Window width={1056} height={680} title={c.window} darkBody={false}>
          {/* REAL Passpine Desktop App UI (full original colors preserved) */}
          <div style={{ position: "absolute", inset: 0, overflow: "hidden" }}>
            <Img
              src={staticFile(`demo/${copy.locale}/desktop.png`)}
              style={{
                width: "100%",
                height: "100%",
                objectFit: "cover",
                objectPosition: "top left",
                transform: `scale(${bgScale})`,
                transformOrigin: "30% 35%",
              }}
            />
            {/* Soft dark scrim over the right detail area so the generator sheet reads crisply while leaving the real sidebar & entry list clear */}
            <div
              style={{
                position: "absolute",
                inset: 0,
                background:
                  "linear-gradient(90deg, rgba(28, 28, 33, 0.06) 0%, rgba(28, 28, 33, 0.28) 42%, rgba(28, 28, 33, 0.62) 100%)",
                opacity: modalIn,
              }}
            />
          </div>

          {/* Monochrome New Entry + Password Generator sheet anchored over the right side of the real desktop UI */}
          <div
            style={{
              position: "absolute",
              right: 28,
              top: 26,
              width: 540,
              padding: "26px 28px",
              borderRadius: 18,
              background: palette.panelGlass,
              border: `1px solid ${palette.silverBorder}`,
              boxShadow: "0 30px 80px rgba(0, 0, 0, 0.55)",
              opacity: modalIn,
              transform: `translateX(${(1 - modalIn) * 28}px)`,
            }}
          >
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 18 }}>
              <div style={{ fontSize: 22, fontWeight: 700, color: palette.ink }}>{c.modalTitle}</div>
              <span
                style={{
                  padding: "4px 10px",
                  borderRadius: 999,
                  background: palette.panelElevated,
                  border: `1px solid ${palette.borderStrong}`,
                  fontFamily: fonts.mono,
                  fontSize: 13,
                  color: palette.silver,
                }}
              >
                {c.entropy}
              </span>
            </div>

            <Field label={c.nameLabel} active={frame >= 24 && frame < 42}>
              {name}
            </Field>
            <Field label={c.usernameLabel} active={frame >= 42 && frame < 62}>
              {username}
            </Field>
            <Field label={c.passwordLabel} active={frame >= 62 && frame < 118}>
              {filledIn ? (
                <span style={{ fontFamily: fonts.mono, fontSize: 18, letterSpacing: 0.8, color: palette.ink }}>
                  {c.password}
                </span>
              ) : (
                <span style={{ color: palette.inkSubtle }}>{c.passwordPlaceholder}</span>
              )}
              <span
                style={{
                  marginLeft: "auto",
                  fontSize: 14,
                  fontWeight: 600,
                  padding: "5px 11px",
                  borderRadius: 7,
                  background: palette.panelElevated,
                  border: `1px solid ${palette.borderStrong}`,
                  color: palette.ink,
                  whiteSpace: "nowrap",
                }}
              >
                {c.generateButton}
              </span>
            </Field>

            {/* Live password generator block */}
            <div
              style={{
                marginTop: 14,
                padding: "16px 18px",
                borderRadius: 12,
                background: palette.panelElevated,
                border: `1px solid ${palette.borderStrong}`,
                opacity: genPanel,
                transform: `translateY(${(1 - genPanel) * 10}px)`,
              }}
            >
              <div
                style={{
                  padding: "12px 14px",
                  borderRadius: 9,
                  background: palette.graphite,
                  border: `1px solid ${palette.border}`,
                  fontFamily: fonts.mono,
                  fontSize: 19,
                  letterSpacing: 1.1,
                  textAlign: "center",
                  color: palette.ink,
                }}
              >
                {scramble(c.password, frame, 94)}
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: 14, marginTop: 14, fontSize: 15 }}>
                <span style={{ color: palette.inkMuted }}>{c.length}</span>
                <div style={{ flex: 1, height: 4, borderRadius: 2, background: palette.graphite, position: "relative" }}>
                  <div style={{ width: "62%", height: "100%", borderRadius: 2, background: palette.ink }} />
                  <div
                    style={{
                      position: "absolute",
                      left: "62%",
                      top: -6,
                      width: 16,
                      height: 16,
                      borderRadius: 8,
                      background: palette.ink,
                      border: `2px solid ${palette.graphite}`,
                      transform: "translateX(-50%)",
                    }}
                  />
                </div>
                <span style={{ fontFamily: fonts.mono, fontWeight: 600, color: palette.ink }}>{c.lengthValue}</span>
                <span style={{ color: palette.inkMuted, marginLeft: 8 }}>{c.symbols}</span>
                <div
                  style={{
                    width: 38,
                    height: 22,
                    borderRadius: 11,
                    background: palette.ink,
                    position: "relative",
                  }}
                >
                  <div
                    style={{
                      position: "absolute",
                      right: 3,
                      top: 3,
                      width: 16,
                      height: 16,
                      borderRadius: 8,
                      background: palette.graphite,
                    }}
                  />
                </div>
              </div>
            </div>

            <div
              style={{
                marginTop: 20,
                height: 48,
                borderRadius: 10,
                background: palette.ink,
                color: palette.graphite,
                display: "grid",
                placeItems: "center",
                fontSize: 18,
                fontWeight: 700,
                transform: `scale(${savePressed ? 0.97 : 1})`,
              }}
            >
              {c.save}
            </div>
          </div>

          <Toast text={c.saved} progress={saved} />
        </Window>
      </div>
    </div>
  );
};
