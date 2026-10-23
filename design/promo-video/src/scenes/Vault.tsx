import React from "react";
import { interpolate, useCurrentFrame } from "remotion";
import { Cursor, FeatureCard, Window } from "../components";
import { copy } from "../copy";
import { fonts, ink, muted, pop, ramp, sceneFade, timing } from "../theme";
import { VaultList } from "./VaultList";

const c = copy.vault;

export const Vault = () => {
  const frame = useCurrentFrame();
  const enter = ramp(frame, 0, 26);
  const importTab = frame >= 33;
  const panel = ramp(frame, 36, 54);
  const pressed = frame >= 96 && frame < 101;
  const opened = ramp(frame, 106, 126);

  const cx = interpolate(frame, [16, 30, 70, 94], [700, 490, 490, 490], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const cy = interpolate(frame, [16, 30, 70, 94], [300, 52, 52, 468], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const cursorOpacity = interpolate(frame, [12, 18, 102, 108], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const clicking = (frame >= 30 && frame < 35) || pressed;

  return (
    <div style={{ position: "absolute", inset: 0, opacity: sceneFade(frame, timing.vault.duration) }}>
      <FeatureCard index={4} title={c.title} caption={c.caption} />
      <div style={{ position: "absolute", left: 800, top: 190, opacity: enter, transform: `translateY(${(1 - enter) * 40}px)` }}>
        <Window width={1000} height={640} title={frame >= 116 ? c.vaultName : c.window}>
          {/* Import screen */}
          <div style={{ position: "absolute", inset: 0, opacity: 1 - opened }}>
            <div style={{ position: "absolute", left: 220, top: 32, width: 560, height: 50, borderRadius: 12, background: "#f0f0ee", display: "flex", padding: 4 }}>
              {c.tabs.map((tab, i) => {
                const on = importTab ? i === 1 : i === 0;
                return (
                  <div
                    key={tab}
                    style={{
                      flex: 1,
                      display: "grid",
                      placeItems: "center",
                      borderRadius: 9,
                      fontSize: 18,
                      fontWeight: on ? 600 : 400,
                      background: on ? "#fff" : "transparent",
                      boxShadow: on ? "0 1px 3px rgba(0,0,0,0.1)" : "none",
                      color: on ? ink : muted,
                    }}
                  >
                    {tab}
                  </div>
                );
              })}
            </div>
            <div style={{ position: "absolute", left: 120, top: 110, width: 760, opacity: panel, transform: `translateY(${(1 - panel) * 12}px)` }}>
              <div style={{ padding: "14px 18px", borderRadius: 12, background: "#eef5ff", color: "#24508f", fontSize: 18 }}>{c.found}</div>
              <div style={{ marginTop: 18, padding: "12px 18px", borderRadius: 12, border: "1px solid #ececea", fontFamily: fonts.mono, fontSize: 17 }}>
                {c.files.map((f, i) => (
                  <div key={f} style={{ height: 30, display: "flex", alignItems: "center", color: i === 0 ? muted : ink, opacity: ramp(frame, 44 + i * 5, 54 + i * 5) }}>
                    {f}
                  </div>
                ))}
              </div>
              <div style={{ marginTop: 22, display: "flex", alignItems: "center", gap: 18, fontSize: 18 }}>
                <span style={{ color: muted }}>{c.keySource}</span>
                <div style={{ display: "flex", padding: 3, borderRadius: 10, background: "#f0f0ee" }}>
                  {c.sources.map((s, i) => (
                    <div key={s} style={{ padding: "8px 18px", borderRadius: 8, background: i === 0 ? "#fff" : "transparent", fontWeight: i === 0 ? 600 : 400, color: i === 0 ? ink : muted }}>
                      {s}
                    </div>
                  ))}
                </div>
              </div>
              <div
                style={{
                  marginTop: 24,
                  height: 56,
                  borderRadius: 12,
                  background: ink,
                  color: "#fff",
                  display: "grid",
                  placeItems: "center",
                  fontSize: 20,
                  fontWeight: 600,
                  transform: `scale(${pressed ? 0.98 : 1})`,
                }}
              >
                {c.importButton}
              </div>
            </div>
          </div>
          {/* Imported vault */}
          <div style={{ position: "absolute", inset: 0, display: "flex", opacity: opened, transform: `translateY(${(1 - opened) * 20}px)` }}>
            <div style={{ width: 380, borderRight: "1px solid #ececea", background: "#fbfbfa" }}>
              <VaultList title={c.allEntries} entries={c.entries} selected={frame >= 130 ? "github" : undefined} />
            </div>
            <div style={{ flex: 1, padding: "30px 40px", opacity: pop(frame, 130) }}>
              <div style={{ fontSize: 34, fontWeight: 700 }}>{c.entries[1].name}</div>
              <div style={{ fontSize: 16, color: muted, marginTop: 6, fontFamily: fonts.mono }}>{c.entries[1].path}.gpg</div>
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
