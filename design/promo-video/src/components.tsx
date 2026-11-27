import React from "react";
import type { CSSProperties, ReactNode } from "react";
import { AbsoluteFill, Img, interpolate, staticFile, useCurrentFrame } from "remotion";
import { fonts, palette, ramp, timing } from "./theme";

/** Passpine Monochrome graphite studio backdrop with subtle architectural grid and silver specular light. */
export const Backdrop = ({ staticFrame }: { staticFrame?: number }) => {
  const liveFrame = useCurrentFrame();
  const frame = staticFrame ?? liveFrame;
  const drift = interpolate(frame, [0, timing.total], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <AbsoluteFill
      style={{
        background: `radial-gradient(1240px 760px at ${20 + drift * 8}% ${16 + drift * 6}%, rgba(196, 196, 204, 0.11) 0%, rgba(43, 44, 48, 0.42) 44%, transparent 74%),
                     radial-gradient(980px 680px at ${84 - drift * 8}% ${84 - drift * 6}%, rgba(196, 196, 204, 0.08) 0%, rgba(37, 38, 44, 0.34) 40%, transparent 72%),
                     linear-gradient(155deg, ${palette.graphite} 0%, #18181d 54%, ${palette.graphiteDeep} 100%)`,
        overflow: "hidden",
      }}
    >
      {/* Subtle architectural coordinate grid */}
      <div
        style={{
          position: "absolute",
          inset: 0,
          backgroundImage: `linear-gradient(to right, rgba(237, 237, 241, 0.032) 1px, transparent 1px),
                            linear-gradient(to bottom, rgba(237, 237, 241, 0.032) 1px, transparent 1px)`,
          backgroundSize: "72px 72px",
          maskImage: "radial-gradient(ellipse 82% 74% at 52% 48%, #000 32%, transparent 100%)",
          WebkitMaskImage: "radial-gradient(ellipse 82% 74% at 52% 48%, #000 32%, transparent 100%)",
        }}
      />
      {/* Top horizon silver specular hairline */}
      <div
        style={{
          position: "absolute",
          left: 140,
          right: 140,
          top: 0,
          height: 2,
          background: `linear-gradient(90deg, transparent 0%, ${palette.silverBorder} 34%, ${palette.ink} 50%, ${palette.silverBorder} 66%, transparent 100%)`,
          opacity: 0.42,
        }}
      />
    </AbsoluteFill>
  );
};

/** Passpine icon inside a refined monochrome graphite/silver emblem tile. */
export const Logo = ({ size }: { size: number }) => (
  <div
    style={{
      width: size,
      height: size,
      borderRadius: size * 0.24,
      background: `linear-gradient(160deg, ${palette.panelElevated} 0%, ${palette.graphite} 100%)`,
      border: `1.5px solid ${palette.silverBorder}`,
      boxShadow: `0 18px 44px rgba(0, 0, 0, 0.45), inset 0 1px 0 rgba(237, 237, 241, 0.22)`,
      display: "grid",
      placeItems: "center",
      flexShrink: 0,
    }}
  >
    <Img src={staticFile("icon.svg")} width={size * 0.86} height={size * 0.86} />
  </div>
);

/** Left editorial column: numbered silver index pill, precision display title, caption, and spec chip. */
export const FeatureCard = ({
  index,
  eyebrow,
  title,
  caption,
  spec,
}: {
  index: number;
  eyebrow: string;
  title: string;
  caption: string;
  spec: string;
}) => {
  const frame = useCurrentFrame();
  const rise = (delay: number): CSSProperties => {
    const t = ramp(frame, delay, delay + 20);
    return { opacity: t, transform: `translateY(${(1 - t) * 22}px)` };
  };

  return (
    <div
      style={{
        position: "absolute",
        left: 112,
        top: 264,
        width: 590,
        color: palette.ink,
        fontFamily: fonts.sans,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 14, ...rise(2) }}>
        <span
          style={{
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            padding: "5px 12px",
            borderRadius: 999,
            background: palette.silverSoft,
            border: `1px solid ${palette.silverBorder}`,
            color: palette.ink,
            fontFamily: fonts.mono,
            fontSize: 16,
            fontWeight: 700,
            letterSpacing: 1.5,
            fontVariantNumeric: "tabular-nums",
          }}
        >
          {String(index).padStart(2, "0")} / 04
        </span>
        <span
          style={{
            fontFamily: fonts.mono,
            fontSize: 15,
            fontWeight: 600,
            letterSpacing: 2.6,
            color: palette.inkMuted,
          }}
        >
          {eyebrow}
        </span>
      </div>

      <div
        style={{
          fontSize: 66,
          fontWeight: 700,
          lineHeight: 1.14,
          marginTop: 26,
          letterSpacing: -1.8,
          color: palette.ink,
          ...rise(8),
        }}
      >
        {title}
      </div>

      <div
        style={{
          fontSize: 29,
          lineHeight: 1.58,
          marginTop: 26,
          color: palette.inkMuted,
          whiteSpace: "pre-line",
          fontWeight: 400,
          ...rise(16),
        }}
      >
        {caption}
      </div>

      <div
        style={{
          marginTop: 36,
          display: "inline-flex",
          alignItems: "center",
          gap: 10,
          padding: "10px 18px",
          borderRadius: 10,
          background: palette.panel,
          border: `1px solid ${palette.border}`,
          fontFamily: fonts.mono,
          fontSize: 16,
          color: palette.silver,
          letterSpacing: 0.4,
          ...rise(24),
        }}
      >
        <span
          style={{
            width: 8,
            height: 8,
            borderRadius: 4,
            background: palette.silver,
            boxShadow: `0 0 10px ${palette.silverGlow}`,
          }}
        />
        {spec}
      </div>
    </div>
  );
};

/** Elevated monochrome graphite/panel window chrome for staging real product UI and browser scenes. */
export const Window = ({
  width,
  height,
  title,
  children,
  toolbar,
  darkBody = true,
}: {
  width: number;
  height: number;
  title?: ReactNode;
  children: ReactNode;
  toolbar?: ReactNode;
  darkBody?: boolean;
}) => (
  <div
    style={{
      width,
      height,
      borderRadius: 20,
      overflow: "hidden",
      background: darkBody ? palette.panel : "#FFFFFF",
      border: `1px solid ${palette.borderStrong}`,
      boxShadow: `0 44px 110px rgba(0, 0, 0, 0.56), 0 0 0 1px rgba(237, 237, 241, 0.06), inset 0 1px 0 rgba(255, 255, 255, 0.08)`,
      fontFamily: fonts.sans,
      color: palette.ink,
      display: "flex",
      flexDirection: "column",
    }}
  >
    <div
      style={{
        height: 50,
        flexShrink: 0,
        display: "flex",
        alignItems: "center",
        gap: 9,
        padding: "0 20px",
        background: `linear-gradient(180deg, ${palette.windowChromeTop} 0%, ${palette.windowChrome} 100%)`,
        borderBottom: `1px solid ${palette.windowChromeBorder}`,
      }}
    >
      {[0.24, 0.18, 0.14].map((op, idx) => (
        <div
          key={idx}
          style={{
            width: 11,
            height: 11,
            borderRadius: 6,
            background: `rgba(237, 237, 241, ${op})`,
            border: "1px solid rgba(237, 237, 241, 0.16)",
          }}
        />
      ))}
      <div
        style={{
          marginLeft: 16,
          fontSize: 15,
          fontWeight: 500,
          color: palette.silver,
          flex: 1,
          letterSpacing: 0.2,
        }}
      >
        {title}
      </div>
      {toolbar}
    </div>
    <div style={{ flex: 1, position: "relative", overflow: "hidden" }}>{children}</div>
  </div>
);

export const Check = ({ size = 20, color = palette.ink }: { size?: number; color?: string }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2.6} strokeLinecap="round" strokeLinejoin="round">
    <path d="M5 12.5l4.5 4.5L19 7.5" />
  </svg>
);

export const LockIcon = ({ size = 18, color = palette.silver }: { size?: number; color?: string }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
    <rect x="4" y="11" width="16" height="10" rx="2.5" />
    <path d="M8 11V7a4 4 0 0 1 8 0v4" />
  </svg>
);

export const GitBranchIcon = ({ size = 18, color = palette.silver }: { size?: number; color?: string }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
    <circle cx="6" cy="6" r="3" />
    <circle cx="18" cy="6" r="3" />
    <circle cx="6" cy="18" r="3" />
    <path d="M6 9v6" />
    <path d="M18 9a6 6 0 0 1-6 6H9" />
  </svg>
);

export const Cursor = ({ x, y, pressed }: { x: number; y: number; pressed: boolean }) => (
  <div style={{ position: "absolute", left: x, top: y, pointerEvents: "none" }}>
    {pressed && (
      <div
        style={{
          position: "absolute",
          left: -14,
          top: -14,
          width: 34,
          height: 34,
          borderRadius: 17,
          border: `2.5px solid ${palette.ink}`,
          background: palette.silverSoft,
          boxShadow: `0 0 22px ${palette.silverGlow}`,
        }}
      />
    )}
    <svg
      width={34}
      height={34}
      viewBox="0 0 24 24"
      style={{
        transform: `scale(${pressed ? 0.86 : 1})`,
        filter: "drop-shadow(0 4px 10px rgba(0, 0, 0, 0.55))",
      }}
    >
      <path d="M4 2l16 10-7 1.5L9.5 21z" fill={palette.ink} stroke={palette.graphiteDeep} strokeWidth={1.6} strokeLinejoin="round" />
    </svg>
  </div>
);

/** Frosted monochrome panel confirmation toast. */
export const Toast = ({ text, progress, style }: { text: string; progress: number; style?: CSSProperties }) => (
  <div
    style={{
      position: "absolute",
      left: "50%",
      bottom: 26,
      transform: `translate(-50%, ${(1 - progress) * 18}px)`,
      opacity: progress,
      display: "flex",
      alignItems: "center",
      gap: 10,
      padding: "12px 22px",
      borderRadius: 999,
      background: "rgba(28, 28, 33, 0.96)",
      border: `1px solid ${palette.silverBorder}`,
      color: palette.ink,
      fontSize: 18,
      fontWeight: 500,
      whiteSpace: "nowrap",
      boxShadow: `0 18px 44px rgba(0, 0, 0, 0.52), 0 0 24px rgba(196, 196, 204, 0.12)`,
      ...style,
    }}
  >
    <Check size={19} color={palette.ink} />
    <span>{text}</span>
  </div>
);

/** Bottom chapter progress bar with live silver/bright fill. */
export const FeatureBar = ({
  labels,
  active,
  activeProgress,
  opacity,
}: {
  labels: string[];
  active: number;
  activeProgress: number;
  opacity: number;
}) => (
  <div
    style={{
      position: "absolute",
      left: 112,
      right: 112,
      bottom: 56,
      display: "flex",
      gap: 20,
      opacity,
      fontFamily: fonts.sans,
    }}
  >
    {labels.map((label, i) => {
      const on = i === active;
      const done = i < active;
      const fillPct = done ? 100 : on ? Math.max(0, Math.min(100, activeProgress * 100)) : 0;
      return (
        <div key={label} style={{ flex: 1 }}>
          <div
            style={{
              height: 3,
              borderRadius: 2,
              background: "rgba(237, 237, 241, 0.13)",
              overflow: "hidden",
            }}
          >
            <div
              style={{
                width: `${fillPct}%`,
                height: "100%",
                borderRadius: 2,
                background: on ? palette.ink : palette.silver,
                opacity: on ? 1 : 0.45,
                boxShadow: on ? `0 0 12px ${palette.silverGlow}` : "none",
              }}
            />
          </div>
          <div
            style={{
              marginTop: 12,
              display: "flex",
              alignItems: "center",
              gap: 10,
              fontSize: 19,
              fontWeight: on ? 600 : 400,
              color: on ? palette.ink : done ? palette.silver : palette.inkMuted,
            }}
          >
            <span
              style={{
                fontFamily: fonts.mono,
                fontSize: 14,
                color: on ? palette.ink : palette.inkMuted,
                fontVariantNumeric: "tabular-nums",
              }}
            >
              {String(i + 1).padStart(2, "0")}
            </span>
            <span>{label}</span>
          </div>
        </div>
      );
    })}
  </div>
);

/** Typed-out text: shows the first n characters for the given local frame window. */
export const typed = (text: string, frame: number, start: number, end: number) =>
  text.slice(
    0,
    Math.round(
      interpolate(frame, [start, end], [0, text.length], {
        extrapolateLeft: "clamp",
        extrapolateRight: "clamp",
      }),
    ),
  );
