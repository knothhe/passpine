import React from "react";
import { Composition } from "remotion";
import { Promo } from "./Promo";
import { FPS, timing } from "./theme";

export const Root = () => (
  <Composition id="Promo" component={Promo} durationInFrames={timing.total} fps={FPS} width={1920} height={1080} />
);
