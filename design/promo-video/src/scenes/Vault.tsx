import React from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { Cursor, FeatureCard, LockIcon, Toast, Window } from "../components";
import { useCopy } from "../copy";
import { fonts, palette, pop, ramp, sceneFade, timing } from "../theme";

export const Vault = () => {
  const copy = useCopy();
  const c = copy.vault;
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 24);
  const importTab = frame >= 30;
  const panel = ramp(frame, 32, 50);
  const pressed = frame >= 88 && frame < 94;
  const opened = ramp(frame, 98, 118);
  const toast = Math.min(pop(frame, 118), 1);

  const cx = interpolate(frame, [14, 28, 66, 86], [760, 528, 528, 528], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const cy = interpolate(frame, [14, 28, 66, 86], [300, 58, 58, 512], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const cursorOpacity = interpolate(frame, [10, 16, 94, 102], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const clicking = (frame >= 28 && frame < 33) || pressed;

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.vault.duration) }}>
      <FeatureCard index={4} eyebrow={c.eyebrow} title={c.title} caption={c.caption} spec={c.spec} />

      <div
        style={{
          position: "absolute",
          left: 752,
          top: 156,
          opacity: enter,
          transform: `translateY(${(1 - enter) * 36}px)`,
        }}
      >
        <Window width={1056} height={680} title={frame >= 108 ? c.importedWindow : c.window} darkBody={frame < 108}>
          {/* Phase 1: Standard pass vault inspector & import flow */}
          <div
            style={{
              position: "absolute",
              inset: 0,
              background: palette.graphite,
              opacity: 1 - opened,
            }}
          >
            {/* Segmented tabs */}
            <div
              style={{
                position: "absolute",
                left: 238,
                top: 32,
                width: 580,
                height: 50,
                borderRadius: 12,
                background: palette.panel,
                border: `1px solid ${palette.border}`,
                display: "flex",
                padding: 4,
              }}
            >
              {c.tabs.map((tab, i) => {
                const on = importTab ? i === 1 : i === 0;
                return (
                  <div
                    key={tab}
                    style={{
                      flex: 1,
                      display: "grid",
                      placeItems: "center",
                      borderRadius: 8,
                      fontSize: 17,
                      fontWeight: on ? 600 : 400,
                      background: on ? palette.panelElevated : "transparent",
                      border: on ? `1px solid ${palette.borderStrong}` : "1px solid transparent",
                      color: on ? palette.ink : palette.inkMuted,
                    }}
                  >
                    {tab}
                  </div>
                );
              })}
            </div>

            <div
              style={{
                position: "absolute",
                left: 128,
                top: 112,
                width: 800,
                opacity: panel,
                transform: `translateY(${(1 - panel) * 12}px)`,
              }}
            >
              {/* Detected vault banner */}
              <div
                style={{
                  minHeight: 50,
                  padding: "12px 18px",
                  borderRadius: 12,
                  background: palette.panel,
                  border: `1px solid ${palette.silverBorder}`,
                  color: palette.ink,
                  fontSize: 16,
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                }}
              >
                <LockIcon size={16} color={palette.silver} />
                <span>{c.found}</span>
              </div>

              {/* Standard ~/.password-store file tree */}
              <div
                style={{
                  marginTop: 18,
                  padding: "16px 20px",
                  borderRadius: 14,
                  background: palette.panel,
                  border: `1px solid ${palette.border}`,
                  fontFamily: fonts.mono,
                  fontSize: 16,
                }}
              >
                {c.files.map((f, i) => (
                  <div
                    key={f.name}
                    style={{
                      height: 36,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      borderBottom: i < c.files.length - 1 ? `1px solid ${palette.border}` : "none",
                      color: f.encrypted ? palette.ink : palette.inkMuted,
                      opacity: ramp(frame, 38 + i * 5, 48 + i * 5),
                    }}
                  >
                    <span>{f.name}</span>
                    {f.encrypted && (
                      <span
                        style={{
                          padding: "2px 9px",
                          borderRadius: 6,
                          background: palette.panelElevated,
                          border: `1px solid ${palette.borderStrong}`,
                          fontSize: 12,
                          color: palette.silver,
                        }}
                      >
                        {c.encryptedTag}
                      </span>
                    )}
                  </div>
                ))}
              </div>

              {/* Key source selector */}
              <div style={{ marginTop: 20, display: "flex", alignItems: "center", gap: 18, fontSize: 17 }}>
                <span style={{ color: palette.inkMuted }}>{c.keySource}</span>
                <div
                  style={{
                    display: "flex",
                    padding: 3,
                    borderRadius: 10,
                    background: palette.panel,
                    border: `1px solid ${palette.border}`,
                  }}
                >
                  {c.sources.map((s, i) => (
                    <div
                      key={s}
                      style={{
                        padding: "7px 18px",
                        borderRadius: 8,
                        background: i === 0 ? palette.panelElevated : "transparent",
                        fontWeight: i === 0 ? 600 : 400,
                        color: i === 0 ? palette.ink : palette.inkMuted,
                      }}
                    >
                      {s}
                    </div>
                  ))}
                </div>
              </div>

              <div
                style={{
                  marginTop: 22,
                  height: 54,
                  borderRadius: 12,
                  background: palette.ink,
                  color: palette.graphite,
                  display: "grid",
                  placeItems: "center",
                  fontSize: 19,
                  fontWeight: 700,
                  transform: `scale(${pressed ? 0.98 : 1})`,
                }}
              >
                {c.importButton}
              </div>
            </div>
          </div>

          {/* Phase 2: Seamless reveal of REAL Passpine Desktop App UI with imported vault open */}
          <div
            style={{
              position: "absolute",
              inset: 0,
              opacity: opened,
              transform: `scale(${0.98 + 0.02 * opened})`,
            }}
          >
            <Img
              src={staticFile(`demo/${copy.locale}/desktop.png`)}
              style={{
                width: "100%",
                height: "100%",
                objectFit: "cover",
                objectPosition: "top left",
              }}
            />
            <Toast text={c.importedToast} progress={toast} />
          </div>

          <div style={{ opacity: cursorOpacity }}>
            <Cursor x={cx} y={cy} pressed={clicking} />
          </div>
        </Window>
      </div>
    </div>
  );
};
