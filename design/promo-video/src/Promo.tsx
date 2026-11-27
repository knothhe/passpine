import React from "react";
import { AbsoluteFill, Audio, interpolate, Sequence, staticFile, useCurrentFrame } from "remotion";
import { Backdrop, FeatureBar } from "./components";
import { CopyContext, dictionaries, type Locale } from "./copy";
import { Intro, Outro } from "./scenes/Bookends";
import { Fill } from "./scenes/Fill";
import { Generate } from "./scenes/Generate";
import { Sync } from "./scenes/Sync";
import { Vault } from "./scenes/Vault";
import { features, fonts, palette, timing } from "./theme";

// scripts/music.mjs masters public/music.wav with final gain staging and fade envelopes;
// keep unity gain here so Remotion renders match the mastered score without double-attenuation.
const MUSIC_VOLUME = 1;

const scenes = [
  [timing.intro, Intro],
  [timing.fill, Fill],
  [timing.generate, Generate],
  [timing.sync, Sync],
  [timing.vault, Vault],
  [timing.outro, Outro],
] as const;

export const Promo = ({ locale }: { locale: Locale }) => {
  const copy = dictionaries[locale];
  const frame = useCurrentFrame();
  const active = features.findLastIndex((f) => frame >= f.from);
  const activeScene = active >= 0 ? features[active] : null;
  const activeProgress = activeScene ? (frame - activeScene.from) / activeScene.duration : 0;

  const barOpacity = interpolate(
    frame,
    [timing.fill.from, timing.fill.from + 18, timing.outro.from - 14, timing.outro.from],
    [0, 1, 1, 0],
    {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    },
  );

  return (
    <CopyContext.Provider value={copy}>
      <AbsoluteFill style={{ background: palette.graphite, fontFamily: fonts.sans }}>
        <Backdrop />
        <Audio src={staticFile("music.wav")} volume={MUSIC_VOLUME} />
        {scenes.map(([t, Scene], i) => (
          <Sequence key={i} from={t.from} durationInFrames={t.duration}>
            <Scene />
          </Sequence>
        ))}
        <FeatureBar
          labels={[copy.fill.title, copy.generate.title, copy.sync.title, copy.vault.title]}
          active={active}
          activeProgress={activeProgress}
          opacity={barOpacity}
        />
      </AbsoluteFill>
    </CopyContext.Provider>
  );
};
