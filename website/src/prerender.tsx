import { renderToString } from "react-dom/server";
import { zh } from "./constants/copy";
import { pageDocuments, type PageId } from "./constants/routes";
import { staticLanguage } from "./lib/language";
import { bundledRelease } from "./lib/release-snapshot";
import { HomePage } from "./pages/home";
import { NotFoundPage } from "./pages/not-found";
import { PrivacyPage } from "./pages/privacy";

export { pageDocuments };

// Build-time HTML for crawlers, store reviewers, and visitors without
// JavaScript. It matches the documents' default `lang="zh-CN"`.
export function render(page: PageId): string {
  const props = { c: zh, control: staticLanguage };
  if (page === "privacy")
    return renderToString(<PrivacyPage {...props} language="zh-CN" />);
  if (page === "notFound") return renderToString(<NotFoundPage {...props} />);
  return renderToString(
    <HomePage
      {...props}
      language="zh-CN"
      device={{ platform: null, architecture: "unknown" }}
      releaseState={{
        release: bundledRelease,
        status: "loading",
        retry: () => {},
      }}
      mobile={false}
    />,
  );
}
