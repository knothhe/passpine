import React from "react";
import { AbsoluteFill, Audio, interpolate, Sequence, staticFile, useCurrentFrame } from "remotion";
import { FeatureBar } from "./components";
import { copy } from "./copy";
import { Intro, Outro } from "./scenes/Bookends";
import { Fill } from "./scenes/Fill";
import { Generate } from "./scenes/Generate";
import { Sync } from "./scenes/Sync";
import { Vault } from "./scenes/Vault";
import { features, fonts, paper, timing } from "./theme";

// Music fades in briefly and fades out across the outro.
const musicVolume = (f: number) =>
  interpolate(f, [0, 20, timing.total - 105, timing.total], [0, 0.8, 0.8, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

const scenes = [
  [timing.intro, Intro],
  [timing.fill, Fill],
  [timing.generate, Generate],
  [timing.sync, Sync],
  [timing.vault, Vault],
  [timing.outro, Outro],
] as const;

export const Promo = () => {
  const frame = useCurrentFrame();
  const active = features.findLastIndex((f) => frame >= f.from);
  const barOpacity = interpolate(frame, [timing.fill.from, timing.fill.from + 20, timing.outro.from - 12, timing.outro.from], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <AbsoluteFill style={{ background: paper, fontFamily: fonts.sans }}>
      <Audio src={staticFile("music.wav")} volume={musicVolume} />
      {scenes.map(([t, Scene], i) => (
        <Sequence key={i} from={t.from} durationInFrames={t.duration}>
          <Scene />
        </Sequence>
      ))}
      <FeatureBar labels={[copy.fill.title, copy.generate.title, copy.sync.title, copy.vault.title]} active={active} opacity={barOpacity} />
    </AbsoluteFill>
  );
};
