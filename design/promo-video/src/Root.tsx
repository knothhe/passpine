import React from "react";
import { Composition } from "remotion";
import { Promo } from "./Promo";
import { FPS, timing } from "./theme";

export const Root = () => (
  <>
    <Composition id="Promo-zh-CN" component={Promo} defaultProps={{ locale: "zh-CN" as const }} durationInFrames={timing.total} fps={FPS} width={1920} height={1080} />
    <Composition id="Promo-en" component={Promo} defaultProps={{ locale: "en" as const }} durationInFrames={timing.total} fps={FPS} width={1920} height={1080} />
  </>
);
