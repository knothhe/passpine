import React from "react";
import type { ReactNode } from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { Check, FeatureCard, Toast, Window } from "../components";
import { useCopy } from "../copy";
import { ink, muted, pop, ramp, sceneFade, timing } from "../theme";
import { VaultList } from "./VaultList";


const SyncIcon = ({ angle }: { angle: number }) => (
  <svg width={26} height={26} viewBox="0 0 24 24" fill="none" stroke={ink} strokeWidth={2} strokeLinecap="round" style={{ transform: `rotate(${angle}deg)` }}>
    <path d="M20 11a8 8 0 0 0-14.5-4.5L4 8M4 4v4h4M4 13a8 8 0 0 0 14.5 4.5L20 16M20 20v-4h-4" />
  </svg>
);

const Row = ({ label, children }: { label: string; children: ReactNode }) => (
  <div style={{ padding: "20px 0", borderBottom: "1px solid #ececea" }}>
    <div style={{ fontSize: 16, color: muted, marginBottom: 10 }}>{label}</div>
    <div style={{ display: "flex", alignItems: "center", fontSize: 22 }}>{children}</div>
  </div>
);

export const Sync = () => {
  const c = useCopy().sync;
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 26);
  const syncing = frame >= 18 && frame < 60;
  const angle = interpolate(frame, [18, 60], [0, 720], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const toast = Math.min(pop(frame, 60), 1 - ramp(frame, 130, 142));
  const fresh = ramp(frame, 64, 84);
  const selected = frame >= 92;
  const detail = ramp(frame, 92, 108);

  // TOTP ring counts down through the rest of the scene.
  const remaining = interpolate(frame, [92, 165], [0.82, 0.42], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const r = 13;
  const circ = 2 * Math.PI * r;

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.sync.duration) }}>
      <FeatureCard index={3} title={c.title} caption={c.caption} />
      <div style={{ position: "absolute", left: 800, top: 190, opacity: enter, transform: `translateY(${(1 - enter) * 40}px)` }}>
        <Window
          width={1000}
          height={640}
          title={c.vault}
          toolbar={
            <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 17, color: "#555" }}>
              {syncing ? <span>{c.syncing}</span> : null}
              {frame >= 60 ? <Check size={24} /> : <SyncIcon angle={angle} />}
            </div>
          }
        >
          <div style={{ display: "flex", height: "100%" }}>
            <div style={{ width: 380, borderRight: "1px solid #ececea", background: "#fbfbfa" }}>
              <VaultList title={c.allEntries} entries={c.entries} freshHeight={fresh} selected={selected ? "Figma" : "GitHub"} />
            </div>
            <div style={{ flex: 1, padding: "30px 40px", opacity: selected ? detail : 1 }}>
              {selected ? (
                <>
                  <div style={{ fontSize: 34, fontWeight: 700 }}>{c.entries[0].name}</div>
                  <div style={{ fontSize: 16, color: muted, marginTop: 6 }}>{c.entries[0].path}</div>
                  <Row label={c.usernameLabel}>{c.username}</Row>
                  <Row label={c.totpLabel}>
                    <span style={{ fontSize: 40, fontWeight: 600, letterSpacing: 6, fontVariantNumeric: "tabular-nums" }}>{c.totp}</span>
                    <svg width={34} height={34} viewBox="0 0 34 34" style={{ marginLeft: 20, transform: "rotate(-90deg)" }}>
                      <circle cx={17} cy={17} r={r} fill="none" stroke="#e6e6e3" strokeWidth={4} />
                      <circle cx={17} cy={17} r={r} fill="none" stroke={ink} strokeWidth={4} strokeDasharray={circ} strokeDashoffset={circ * (1 - remaining)} strokeLinecap="round" />
                    </svg>
                    <span style={{ marginLeft: "auto", fontSize: 17, color: "#555" }}>{c.copy}</span>
                  </Row>
                  <Row label={c.passwordLabel}>
                    <span style={{ letterSpacing: 3 }}>••••••••••••</span>
                    <span style={{ marginLeft: "auto", fontSize: 17, color: "#555" }}>{c.copy}</span>
                  </Row>
                </>
              ) : (
                <div style={{ opacity: 0.6 }}>
                  {[260, 140, 480, 360, 480, 300].map((w, i) => (
                    <div key={i} style={{ width: w, height: i === 0 ? 30 : 16, borderRadius: 8, background: "#efefed", marginTop: i === 0 ? 0 : 30 }} />
                  ))}
                </div>
              )}
            </div>
          </div>
          <Toast text={c.synced} progress={toast} />
        </Window>
      </div>
    </div>
  );
};
