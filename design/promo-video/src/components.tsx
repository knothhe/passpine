import React from "react";
import type { CSSProperties, ReactNode } from "react";
import { Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { fonts, ink, muted, ramp } from "./theme";

/** Left column: what the scene shows, stated plainly. */
export const FeatureCard = ({ index, title, caption }: { index: number; title: string; caption: string }) => {
  const frame = useCurrentFrame();
  const rise = (delay: number): CSSProperties => {
    const t = ramp(frame, delay, delay + 22);
    return { opacity: t, transform: `translateY(${(1 - t) * 24}px)` };
  };
  return (
    <div style={{ position: "absolute", left: 140, top: 330, width: 600, color: ink, fontFamily: fonts.sans }}>
      <div style={{ fontSize: 26, fontWeight: 600, color: muted, letterSpacing: 2, fontVariantNumeric: "tabular-nums", ...rise(4) }}>
        {String(index).padStart(2, "0")}
      </div>
      <div style={{ fontSize: 76, fontWeight: 700, lineHeight: 1.2, marginTop: 18, letterSpacing: -1, ...rise(10) }}>{title}</div>
      <div style={{ fontSize: 36, lineHeight: 1.6, marginTop: 28, color: "#4a4a47", whiteSpace: "pre-line", ...rise(20) }}>{caption}</div>
    </div>
  );
};

/** A plain desktop window frame matching the client's neutral look. */
export const Window = ({
  width,
  height,
  title,
  children,
  toolbar,
}: {
  width: number;
  height: number;
  title?: string;
  children: ReactNode;
  toolbar?: ReactNode;
}) => (
  <div
    style={{
      width,
      height,
      borderRadius: 18,
      overflow: "hidden",
      background: "#ffffff",
      boxShadow: "0 40px 100px rgba(20,20,10,0.14), 0 0 0 1px rgba(0,0,0,0.06)",
      fontFamily: fonts.sans,
      color: ink,
      display: "flex",
      flexDirection: "column",
    }}
  >
    <div
      style={{
        height: 56,
        flexShrink: 0,
        display: "flex",
        alignItems: "center",
        gap: 10,
        padding: "0 22px",
        background: "#f2f2f1",
        borderBottom: "1px solid #e4e4e2",
      }}
    >
      {["#ff5f57", "#febc2e", "#28c840"].map((c) => (
        <div key={c} style={{ width: 14, height: 14, borderRadius: 7, background: c }} />
      ))}
      <div style={{ marginLeft: 18, fontSize: 19, fontWeight: 600, flex: 1 }}>{title}</div>
      {toolbar}
    </div>
    <div style={{ flex: 1, position: "relative" }}>{children}</div>
  </div>
);

export const Logo = ({ size }: { size: number }) => <Img src={staticFile("icon.svg")} width={size} height={size} />;

export const Check = ({ size = 22, color = "#1f9d55" }: { size?: number; color?: string }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2.6} strokeLinecap="round" strokeLinejoin="round">
    <path d="M5 12.5l4.5 4.5L19 7.5" />
  </svg>
);

export const Cursor = ({ x, y, pressed }: { x: number; y: number; pressed: boolean }) => (
  <svg
    width={34}
    height={34}
    viewBox="0 0 24 24"
    style={{ position: "absolute", left: x, top: y, transform: `scale(${pressed ? 0.85 : 1})`, filter: "drop-shadow(0 3px 6px rgba(0,0,0,0.25))" }}
  >
    <path d="M4 2l16 10-7 1.5L9.5 21z" fill={ink} stroke="#fff" strokeWidth={1.4} strokeLinejoin="round" />
  </svg>
);

/** Small dark toast used for save and sync confirmations. */
export const Toast = ({ text, progress, style }: { text: string; progress: number; style?: CSSProperties }) => (
  <div
    style={{
      position: "absolute",
      left: "50%",
      bottom: 28,
      transform: `translate(-50%, ${(1 - progress) * 20}px)`,
      opacity: progress,
      display: "flex",
      alignItems: "center",
      gap: 10,
      padding: "12px 22px",
      borderRadius: 24,
      background: ink,
      color: "#fff",
      fontSize: 19,
      whiteSpace: "nowrap",
      boxShadow: "0 12px 30px rgba(0,0,0,0.2)",
      ...style,
    }}
  >
    <Check size={20} color="#7ee2a8" /> {text}
  </div>
);

/** Feature labels along the bottom; the current one is highlighted. */
export const FeatureBar = ({ labels, active, opacity }: { labels: string[]; active: number; opacity: number }) => (
  <div style={{ position: "absolute", left: 140, right: 140, bottom: 80, display: "flex", gap: 16, opacity, fontFamily: fonts.sans }}>
    {labels.map((label, i) => {
      const on = i === active;
      return (
        <div key={label} style={{ flex: 1 }}>
          <div style={{ height: 3, borderRadius: 2, background: ink, opacity: on ? 0.9 : i < active ? 0.35 : 0.12 }} />
          <div style={{ marginTop: 14, fontSize: 21, fontWeight: on ? 600 : 400, color: ink, opacity: on ? 0.9 : 0.4 }}>{label}</div>
        </div>
      );
    })}
  </div>
);

/** Typed-out text: shows the first n characters for the given local frame window. */
export const typed = (text: string, frame: number, start: number, end: number) =>
  text.slice(0, Math.round(interpolate(frame, [start, end], [0, text.length], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })));
