import React from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { Check, FeatureCard, GitBranchIcon, LockIcon, Toast, Window } from "../components";
import { useCopy } from "../copy";
import { fonts, palette, pop, ramp, sceneFade, timing } from "../theme";

const SyncIcon = ({ angle }: { angle: number }) => (
  <svg
    width={20}
    height={20}
    viewBox="0 0 24 24"
    fill="none"
    stroke={palette.ink}
    strokeWidth={2.2}
    strokeLinecap="round"
    style={{ transform: `rotate(${angle}deg)` }}
  >
    <path d="M20 11a8 8 0 0 0-14.5-4.5L4 8M4 4v4h4M4 13a8 8 0 0 0 14.5 4.5L20 16M20 20v-4h-4" />
  </svg>
);

export const Sync = () => {
  const copy = useCopy();
  const c = copy.sync;
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 24);
  const syncing = frame >= 18 && frame < 60;
  const synced = frame >= 60;
  const angle = interpolate(frame, [18, 60], [0, 720], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const toast = Math.min(pop(frame, 60), 1 - ramp(frame, 134, 148));
  const cardIn = pop(frame, 74);
  const zoom = interpolate(frame, [0, 72, timing.sync.duration], [1.0, 1.015, 1.028], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  // TOTP ring counts down through the scene.
  const remaining = interpolate(frame, [74, timing.sync.duration], [0.84, 0.42], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const r = 15;
  const circ = 2 * Math.PI * r;

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.sync.duration) }}>
      <FeatureCard index={3} eyebrow={c.eyebrow} title={c.title} caption={c.caption} spec={c.spec} />

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
          title={c.window}
          darkBody={false}
          toolbar={
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "4px 12px",
                borderRadius: 999,
                background: palette.graphite,
                border: `1px solid ${synced ? palette.silverBorder : palette.border}`,
                fontSize: 14,
                fontFamily: fonts.mono,
                color: synced ? palette.ink : palette.silver,
              }}
            >
              {synced ? <Check size={16} color={palette.ink} /> : <SyncIcon angle={angle} />}
              <span>{syncing ? c.syncing : synced ? c.gitRemote : c.gitRemote}</span>
            </div>
          }
        >
          {/* REAL Passpine Desktop App UI (full original colors preserved) */}
          <div style={{ position: "absolute", inset: 0, overflow: "hidden" }}>
            <Img
              src={staticFile(`demo/${copy.locale}/desktop.png`)}
              style={{
                width: "100%",
                height: "100%",
                objectFit: "cover",
                objectPosition: "top left",
                transform: `scale(${zoom})`,
                transformOrigin: "50% 15%",
              }}
            />
          </div>

          {/* Crisp monochrome callout ring over the real desktop app's Synced / Sync top-right toolbar */}
          <div
            style={{
              position: "absolute",
              right: 26,
              top: 6,
              width: 224,
              height: 38,
              borderRadius: 10,
              border: `2px solid ${palette.graphite}`,
              boxShadow: "0 0 0 4px rgba(28, 28, 33, 0.14)",
              opacity: ramp(frame, 18, 32) * (1 - ramp(frame, 88, 102)),
              pointerEvents: "none",
            }}
          />

          {/* Floating Monochrome Live TOTP & Git Sync Status Card docked bottom-right */}
          <div
            style={{
              position: "absolute",
              right: 28,
              bottom: 76,
              width: 454,
              padding: "22px 24px",
              borderRadius: 18,
              background: palette.panelGlass,
              border: `1px solid ${palette.silverBorder}`,
              boxShadow: "0 28px 70px rgba(0, 0, 0, 0.52)",
              opacity: cardIn,
              transform: `translateY(${(1 - cardIn) * 22}px)`,
            }}
          >
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
              <span
                style={{
                  fontFamily: fonts.mono,
                  fontSize: 13,
                  letterSpacing: 1.5,
                  color: palette.inkMuted,
                  whiteSpace: "nowrap",
                }}
              >
                {c.totpHeader}
              </span>
              <span
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 6,
                  fontSize: 13,
                  fontFamily: fonts.mono,
                  color: palette.silver,
                  whiteSpace: "nowrap",
                }}
              >
                <LockIcon size={13} color={palette.silver} />
                {c.keyNote}
              </span>
            </div>

            <div style={{ marginTop: 10, fontSize: 18, fontWeight: 600, color: palette.silver }}>
              {c.totpAccount}
            </div>

            <div style={{ marginTop: 12, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <span
                style={{
                  fontSize: 42,
                  fontWeight: 700,
                  letterSpacing: 5,
                  fontFamily: fonts.mono,
                  fontVariantNumeric: "tabular-nums",
                  color: palette.ink,
                }}
              >
                {c.totp}
              </span>
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <span style={{ fontFamily: fonts.mono, fontSize: 15, color: palette.inkMuted }}>
                  {c.totpRemaining}
                </span>
                <svg width={38} height={38} viewBox="0 0 38 38" style={{ transform: "rotate(-90deg)" }}>
                  <circle cx={19} cy={19} r={r} fill="none" stroke={palette.panelElevated} strokeWidth={4} />
                  <circle
                    cx={19}
                    cy={19}
                    r={r}
                    fill="none"
                    stroke={palette.ink}
                    strokeWidth={4}
                    strokeDasharray={circ}
                    strokeDashoffset={circ * (1 - remaining)}
                    strokeLinecap="round"
                  />
                </svg>
              </div>
            </div>

            <div
              style={{
                marginTop: 16,
                paddingTop: 14,
                borderTop: `1px solid ${palette.border}`,
                display: "flex",
                alignItems: "center",
                gap: 10,
                fontFamily: fonts.mono,
                fontSize: 14,
                color: palette.silver,
              }}
            >
              <GitBranchIcon size={15} color={palette.silver} />
              <span>{c.gitRemote}</span>
              <span style={{ marginLeft: "auto", color: palette.ink }}>OK</span>
            </div>
          </div>

          <Toast text={c.synced} progress={toast} />
        </Window>
      </div>
    </div>
  );
};
