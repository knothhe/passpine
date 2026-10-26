import { describe, expect, it } from "vitest";
import { MEDIA_ORIGIN, promoVideo } from "./media";

describe("promo video media", () => {
  it.each(["en", "zh-CN"] as const)(
    "serves the %s video and poster from the media origin",
    (language) => {
      const base = `${MEDIA_ORIGIN}/passpine/promo/v1/passpine-promo-${language}`;
      expect(promoVideo(language)).toEqual({
        src: `${base}.mp4`,
        poster: `${base}.jpg`,
      });
    },
  );
});
