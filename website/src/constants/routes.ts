// Each page is a separate static document, so routes need no client router
// or SPA fallback. Cloudflare serves `privacy.html` at `/privacy`.
export type PageId = "home" | "privacy" | "notFound";

export const pageDocuments = {
  home: "index.html",
  privacy: "privacy.html",
  notFound: "404.html",
} as const satisfies Record<PageId, string>;

export const routes = { home: "/", privacy: "/privacy" } as const;

export type HomeSectionId =
  | "video"
  | "features"
  | "security"
  | "guide"
  | "download"
  | "faq";

// Absolute so the same links work from every page; on the home page they are
// same-document fragment navigations.
export const sectionHref = (id: HomeSectionId) => `${routes.home}#${id}`;

export const privacySectionIds = [
  "scope",
  "data",
  "storage",
  "sharing",
  "permissions",
  "website",
  "limits",
  "choices",
  "changes",
  "contact",
] as const;
export type PrivacySectionId = (typeof privacySectionIds)[number];

// The date the policy was last revised, the release it was checked against,
// and the maintainer's private contact address.
export const privacyPolicy = {
  updated: "2026-10-23",
  reviewedVersion: "0.1.7",
  contactEmail: "knothhe@gmail.com",
};
