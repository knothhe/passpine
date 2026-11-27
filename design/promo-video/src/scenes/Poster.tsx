import React from "react";
import { AbsoluteFill, Img, staticFile } from "remotion";
import { Backdrop, Check, Logo, Window } from "../components";
import { CopyContext, dictionaries, type Locale } from "../copy";
import { fonts, palette } from "../theme";

/** Dedicated 1920x1080 hero poster frame for each locale, featuring Passpine's Monochrome identity and both real demo UIs. */
export const Poster = ({ locale }: { locale: Locale }) => {
  const c = dictionaries[locale];

  return (
    <CopyContext.Provider value={c}>
      <AbsoluteFill style={{ fontFamily: fonts.sans, color: palette.ink }}>
        <Backdrop staticFrame={420} />

        {/* Left editorial hero block */}
        <div
          style={{
            position: "absolute",
            left: 108,
            top: 152,
            width: 670,
            display: "flex",
            flexDirection: "column",
            alignItems: "flex-start",
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <Logo size={68} />
            <div>
              <div style={{ fontSize: 34, fontWeight: 700, letterSpacing: -1, lineHeight: 1.05 }}>{c.brand}</div>
              <div
                style={{
                  marginTop: 4,
                  fontFamily: fonts.mono,
                  fontSize: 13,
                  letterSpacing: 2.2,
                  color: palette.inkMuted,
                }}
              >
                {c.poster.filmBadge}
              </div>
            </div>
          </div>

          <div
            style={{
              marginTop: 48,
              padding: "6px 14px",
              borderRadius: 999,
              background: palette.panel,
              border: `1px solid ${palette.borderStrong}`,
              fontFamily: fonts.mono,
              fontSize: 14,
              letterSpacing: 2.4,
              color: palette.silver,
            }}
          >
            {c.eyebrow}
          </div>

          <div
            style={{
              marginTop: 26,
              fontSize: c.locale === "zh-CN" ? 84 : 88,
              fontWeight: 700,
              lineHeight: 1.08,
              letterSpacing: c.locale === "zh-CN" ? -2 : -3.2,
            }}
          >
            <div style={{ color: palette.ink }}>{c.heroTitle}</div>
            <div style={{ color: palette.silver }}>{c.heroAccent}</div>
          </div>

          <div
            style={{
              marginTop: 26,
              fontSize: 26,
              lineHeight: 1.6,
              color: palette.inkMuted,
              maxWidth: 600,
            }}
          >
            {c.description}
          </div>

          <div style={{ marginTop: 44, display: "flex", flexWrap: "wrap", gap: 12 }}>
            {c.outro.points.map((point) => (
              <div
                key={point}
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 8,
                  padding: "9px 16px",
                  borderRadius: 999,
                  background: palette.panel,
                  border: `1px solid ${palette.border}`,
                  fontSize: 17,
                  color: palette.ink,
                }}
              >
                <Check size={15} color={palette.silver} />
                <span>{point}</span>
              </div>
            ))}
          </div>

          <div
            style={{
              marginTop: 32,
              fontFamily: fonts.mono,
              fontSize: 16,
              letterSpacing: 3,
              color: palette.inkSubtle,
            }}
          >
            {c.outro.platforms}
          </div>
        </div>

        {/* Right layered gallery of REAL Desktop + REAL Extension demo UIs (original screenshot colors preserved) */}
        <div style={{ position: "absolute", right: 92, top: 148, width: 970, height: 760 }}>
          <div style={{ transform: "rotate(-1.2deg)" }}>
            <Window width={920} height={636} title={c.generate.window} darkBody={false}>
              <Img
                src={staticFile(`demo/${locale}/desktop.png`)}
                style={{ width: "100%", height: "100%", objectFit: "cover", objectPosition: "top left" }}
              />
            </Window>
          </div>

          {/* Floating REAL Extension popup */}
          <div
            style={{
              position: "absolute",
              right: -12,
              bottom: 34,
              width: 326,
              borderRadius: 16,
              overflow: "hidden",
              background: "#ffffff",
              border: `1px solid ${palette.silverBorder}`,
              boxShadow: "0 32px 84px rgba(0, 0, 0, 0.62)",
              transform: "rotate(2.2deg)",
            }}
          >
            <Img
              src={staticFile(`demo/${locale}/extension.png`)}
              style={{ width: "100%", height: "auto", display: "block" }}
            />
          </div>
        </div>
      </AbsoluteFill>
    </CopyContext.Provider>
  );
};
