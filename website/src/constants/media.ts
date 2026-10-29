// Large media is served from the R2 bucket bound to oss.knothhe.com, not bundled with the site.
// Paths are versioned: upload a new version instead of overwriting, so caches never serve stale video.
export const MEDIA_ORIGIN = "https://oss.knothhe.com";
const PROMO_BASE = `${MEDIA_ORIGIN}/passpine/promo/v1`;

export type MediaLanguage = "en" | "zh-CN";

// Product screenshots live in the repository's root assets/website directory and
// are served under /images by the website-images plugin in vite.config.ts.
export function productScreenshots(language: MediaLanguage) {
  return {
    desktop: `/images/${language}/desktop.png`,
    extension: `/images/${language}/extension.png`,
  };
}

export function promoVideo(language: MediaLanguage) {
  return {
    src: `${PROMO_BASE}/passpine-promo-${language}.mp4`,
    poster: `${PROMO_BASE}/passpine-promo-${language}.jpg`,
  };
}
