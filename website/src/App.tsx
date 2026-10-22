import { useEffect, useState, type ReactNode } from "react";
import { LazyMotion, domAnimation, m, MotionConfig } from "motion/react";
import {
  ArrowDown,
  ArrowDownToLine,
  ArrowRight,
  ArrowUpRight,
  Check,
  ChevronDown,
  CircleCheck,
  Code2,
  Command,
  FileKey2,
  FileLock2,
  Folder,
  GitBranch,
  Globe2,
  KeyRound,
  LockKeyhole,
  Monitor,
  Puzzle,
  RefreshCw,
  ShieldCheck,
  Sprout,
} from "lucide-react";
import { Button } from "./components/ui/button";
import {
  zh,
  en,
  resolveLanguage,
  type Copy,
  type LanguagePreference,
} from "./constants/copy";
import {
  chooseAsset,
  chooseExtension,
  detectDevice,
  detectPlatform,
  fetchLatestRelease,
  GITHUB_URL,
  RELEASES_URL,
  type Device,
  type Platform,
  type Release,
  type ReleaseAsset,
} from "./lib/downloads";

import { bundledRelease } from "./lib/release-snapshot";

const DOCS_URL = `${GITHUB_URL}/blob/main/docs/install-local.md`;
const platformIds: Platform[] = ["macos", "windows", "linux"];

const featureIcons = [ShieldCheck, KeyRound, Puzzle, GitBranch, Code2, Command];

function ExternalLink({
  href,
  children,
  className,
}: {
  href: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className={className}
    >
      {children}
    </a>
  );
}

function useLanguage() {
  const [preference, setPreference] = useState<LanguagePreference>(() => {
    try {
      const saved = localStorage.getItem("passpine-website-language");
      return saved === "en" || saved === "zh-CN" ? saved : "system";
    } catch {
      return "system";
    }
  });
  const [languages, setLanguages] = useState(() => navigator.languages);
  useEffect(() => {
    const update = () => setLanguages(navigator.languages);
    window.addEventListener("languagechange", update);
    return () => window.removeEventListener("languagechange", update);
  }, []);
  const language = resolveLanguage(preference, languages);
  const copy = language === "zh-CN" ? zh : en;
  useEffect(() => {
    document.documentElement.lang = language;
    document.title = copy.meta.title;
    for (const selector of [
      'meta[name="description"]',
      'meta[property="og:description"]',
    ]) {
      document
        .querySelector(selector)
        ?.setAttribute("content", copy.meta.description);
    }
    document
      .querySelector('meta[property="og:title"]')
      ?.setAttribute("content", copy.meta.title);
    try {
      localStorage.setItem("passpine-website-language", preference);
    } catch {
      /* Storage can be disabled. */
    }
  }, [language, copy, preference]);
  return { copy, preference, setPreference };
}

function useRelease() {
  const [release, setRelease] = useState<Release>(bundledRelease);
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading",
  );
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setStatus("loading");
    fetchLatestRelease(controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) {
          setRelease(value);
          setStatus("ready");
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setStatus("error");
      });
    return () => controller.abort();
  }, [attempt]);
  return { release, status, retry: () => setAttempt((value) => value + 1) };
}

function PlatformIcon({
  platform,
  className,
}: {
  platform: Platform;
  className?: string;
}) {
  if (platform === "windows")
    return (
      <svg
        aria-hidden="true"
        className={className}
        viewBox="0 0 24 24"
        fill="currentColor"
      >
        <path d="m2 5 9-1.25V11H2V5Zm11-1.5L22 2v9h-9V3.5ZM2 13h9v7.25L2 19v-6Zm11 0h9v9l-9-1.5V13Z" />
      </svg>
    );
  if (platform === "macos")
    return (
      <svg
        aria-hidden="true"
        className={className}
        viewBox="0 0 24 24"
        fill="currentColor"
      >
        <path d="M17.1 12.5c0-2.2 1.8-3.3 1.9-3.4-1-1.5-2.6-1.7-3.2-1.7-1.4-.2-2.7.8-3.4.8-.7 0-1.8-.8-2.9-.8C8 7.5 6.5 8.4 5.7 9.8c-1.6 2.8-.4 6.9 1.1 9.2.8 1.1 1.6 2.2 2.8 2.1 1.1 0 1.6-.7 3-.7 1.3 0 1.8.7 3 .7s2-1.1 2.7-2.1a10 10 0 0 0 1.3-2.6 4 4 0 0 1-2.5-3.9ZM14.8 5.9c.6-.8 1.1-1.9 1-3-.9 0-2 .6-2.7 1.4-.6.6-1.2 1.8-1 2.8 1 0 2-.5 2.7-1.2Z" />
      </svg>
    );
  return <Monitor aria-hidden="true" className={className} />;
}

function ProductPreview({ c }: { c: Copy }) {
  return (
    <m.figure
      className="product-preview"
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.7, delay: 0.15 }}
    >
      <div className="preview-halo" aria-hidden="true" />
      <div className="desktop-window">
        <div className="window-chrome" aria-hidden="true">
          <div className="window-dots">
            <i />
            <i />
            <i />
          </div>
          <span>{c.brand}</span>
          <LockKeyhole size={10} />
        </div>
        <img
          src="/images/desktop.png"
          alt={c.hero.desktopAlt}
          width="1120"
          height="760"
          fetchPriority="high"
        />
      </div>
      <div className="extension-window">
        <img
          src="/images/extension.png"
          alt={c.hero.extensionAlt}
          width="380"
          height="460"
        />
      </div>
      <figcaption>{c.hero.preview}</figcaption>
    </m.figure>
  );
}

function assetArchitecture(c: Copy, asset: ReleaseAsset): string {
  if (!asset.architecture) return "";
  return asset.platform === "macos"
    ? c.downloads.macArchitectures[asset.architecture]
    : c.downloads.architectures[asset.architecture];
}

export function DownloadSection({
  c,
  device,
  releaseState,
  mobile = false,
}: {
  c: Copy;
  device: Device;
  releaseState: {
    release: Release;
    status: "loading" | "ready" | "error";
    retry: () => void;
  };
  mobile?: boolean;
}) {
  const { release, status, retry } = releaseState;
  const availablePlatforms = platformIds.filter((platform) =>
    release.assets.some((asset) => asset.platform === platform),
  );
  const orderedPlatforms = [...availablePlatforms].sort(
    (a, b) => Number(b === device.platform) - Number(a === device.platform),
  );
  const extensions = (["chrome", "firefox"] as const).flatMap((browser) => {
    const asset = chooseExtension(release, browser);
    return asset ? [{ browser, asset }] : [];
  });
  return (
    <section
      id="download"
      className="download-section section-shell"
      aria-labelledby="download-title"
    >
      <div className="section-heading centered">
        <span className="eyebrow">{c.downloads.eyebrow}</span>
        <h2 id="download-title">{c.downloads.title}</h2>
        <p>{c.downloads.description}</p>
      </div>
      <div className="release-info">
        <span className="release-version">
          {status === "ready"
            ? c.downloads.latest(release.tag)
            : c.downloads.savedVersion(release.tag)}
        </span>
        <div className="release-status" role="status" aria-live="polite">
          {status === "loading" ? (
            <>
              <RefreshCw className="spin" size={13} />
              {c.downloads.loading}
            </>
          ) : status === "error" ? (
            <>
              {c.downloads.error}
              <button type="button" onClick={retry} className="retry-button">
                {c.downloads.retry}
              </button>
            </>
          ) : null}
        </div>
      </div>
      <p className="detected-system">
        {mobile
          ? c.downloads.mobile
          : device.platform
            ? c.downloads.detected(c.downloads.platforms[device.platform])
            : c.downloads.unknown}
      </p>
      <div className="download-options">
        {orderedPlatforms.map((platform) => (
          <article className="platform-download" key={platform}>
            <div className="platform-download-heading">
              <PlatformIcon platform={platform} className="size-6" />
              <h3>{c.downloads.platforms[platform]}</h3>
              {platform === device.platform && (
                <span className="system-badge">
                  {c.downloads.currentSystem}
                </span>
              )}
            </div>
            <div className="platform-download-links">
              {release.assets
                .filter((asset) => asset.platform === platform)
                .sort(
                  (a, b) =>
                    Number(b.architecture === device.architecture) -
                    Number(a.architecture === device.architecture),
                )
                .map((asset) => (
                  <div key={asset.name}>
                    <span className="asset-architecture">
                      {assetArchitecture(c, asset)}
                    </span>
                    <Button asChild className="download-button">
                      <a
                        href={asset.url}
                        aria-label={c.downloads.download(
                          c.downloads.platforms[platform],
                          assetArchitecture(c, asset),
                        )}
                      >
                        <ArrowDownToLine />
                        {c.downloads.download(
                          c.downloads.platforms[platform],
                          assetArchitecture(c, asset),
                        )}
                      </a>
                    </Button>
                  </div>
                ))}
            </div>
            <p className="install-note">{c.downloads.notes[platform]}</p>
          </article>
        ))}
      </div>
      {orderedPlatforms.length === 0 && (
        <p className="install-note">{c.downloads.unavailable}</p>
      )}
      <ExternalLink
        href={DOCS_URL}
        className="text-link small-link install-guide"
      >
        {c.downloads.installGuide}
        <ArrowUpRight size={13} />
      </ExternalLink>
      {extensions.length > 0 && (
        <div className="extension-downloads">
          <div>
            <Puzzle size={22} strokeWidth={1.5} />
            <h3>{c.downloads.extensionTitle}</h3>
            <p>{c.downloads.extensionBody}</p>
          </div>
          <div className="extension-actions">
            {extensions.map(({ browser, asset }) => {
              const name =
                browser === "chrome"
                  ? c.downloads.chromium
                  : c.downloads.firefox;
              return (
                <Button variant="outline" asChild key={browser}>
                  <a
                    href={asset.url}
                    aria-label={c.downloads.extensionDownload(name)}
                  >
                    {name}
                    <ArrowDownToLine />
                  </a>
                </Button>
              );
            })}
            <ExternalLink
              href={`${DOCS_URL}#首次加载与使用`}
              className="text-link small-link"
            >
              {c.downloads.extensionGuide}
              <ArrowUpRight size={13} />
            </ExternalLink>
          </div>
        </div>
      )}
      <ExternalLink href={RELEASES_URL} className="text-link all-releases">
        {c.downloads.releases}
        <ArrowUpRight size={14} />
      </ExternalLink>
    </section>
  );
}

export function App() {
  const { copy: c, preference, setPreference } = useLanguage();
  const [device, setDevice] = useState<Device>(() => detectPlatform());
  const releaseState = useRelease();
  useEffect(() => {
    let active = true;
    detectDevice().then((value) => {
      if (active) setDevice(value);
    });
    return () => {
      active = false;
    };
  }, []);
  const recommended = chooseAsset(
    releaseState.release,
    device.platform,
    device.architecture,
  );
  const navItems = ["features", "security", "guide", "faq"] as const;
  return (
    <LazyMotion features={domAnimation}>
      <MotionConfig reducedMotion="user">
        <a className="skip-link" href="#main">
          {c.skip}
        </a>
        <header className="site-header">
          <a href="#" className="brand" aria-label={c.brand}>
            <img src="/icon.svg" width="43" height="43" alt="" />
            <span>{c.brand}</span>
          </a>
          <nav aria-label={c.nav.label}>
            {navItems.map((id) => (
              <a key={id} href={`#${id}`}>
                {c.nav[id]}
              </a>
            ))}
          </nav>
          <div className="header-actions">
            <div className="language-select">
              <Globe2 size={14} aria-hidden="true" />
              <select
                aria-label={c.language.label}
                value={preference}
                onChange={(event) =>
                  setPreference(event.target.value as LanguagePreference)
                }
              >
                <option value="system">{c.language.system}</option>
                <option value="en">{c.language.en}</option>
                <option value="zh-CN">{c.language.zh}</option>
              </select>
            </div>
            <ExternalLink href={GITHUB_URL} className="github-link">
              <GitBranch size={17} />
              <span>{c.nav.source}</span>
              <ArrowUpRight size={13} />
            </ExternalLink>
          </div>
        </header>
        <main id="main">
          <section className="hero" aria-labelledby="hero-title">
            <m.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.6 }}
            >
              <div className="eyebrow hero-eyebrow">
                <span className="status-dot" />
                {c.hero.eyebrow}
              </div>
              <h1 id="hero-title">{c.hero.title}</h1>
              <p className="hero-description">{c.hero.description}</p>
              <div className="hero-actions">
                <Button asChild>
                  <a href={recommended?.url ?? "#download"}>
                    {recommended && device.platform ? (
                      <PlatformIcon
                        platform={device.platform}
                        className="size-4"
                      />
                    ) : (
                      <ArrowDownToLine />
                    )}
                    {recommended && device.platform
                      ? c.hero.downloadFor(
                          c.downloads.platforms[device.platform],
                          assetArchitecture(c, recommended),
                        )
                      : c.hero.download}
                  </a>
                </Button>
                <a href="#features" className="text-link">
                  {c.hero.explore}
                  <ArrowRight size={15} />
                </a>
              </div>
              <p className="platform-support">{c.hero.platforms}</p>
            </m.div>
            <ProductPreview c={c} />
            <div className="hero-points">
              {c.hero.points.map((text) => (
                <span key={text}>
                  <CircleCheck size={13} />
                  {text}
                </span>
              ))}
            </div>
          </section>
          <section
            id="features"
            className="features-section section-shell"
            aria-labelledby="features-title"
          >
            <div className="section-heading split-heading">
              <div>
                <span className="eyebrow">{c.features.eyebrow}</span>
                <h2 id="features-title">{c.features.title}</h2>
              </div>
              <p>{c.features.description}</p>
            </div>
            <div className="feature-grid">
              {c.features.items.map((item, index) => {
                const Icon = featureIcons[index];
                return (
                  <article key={item.tag}>
                    <div className="feature-top">
                      <Icon size={25} strokeWidth={1.4} />
                      <span>{String(index + 1).padStart(2, "0")}</span>
                    </div>
                    <h3>{item.title}</h3>
                    <p>{item.body}</p>
                    <span className="feature-tag">{item.tag}</span>
                  </article>
                );
              })}
            </div>
          </section>
          <section
            id="security"
            className="security-section"
            aria-labelledby="security-title"
          >
            <div className="section-shell security-inner">
              <div className="security-copy">
                <span className="eyebrow">{c.security.eyebrow}</span>
                <h2 id="security-title">{c.security.title}</h2>
                <p>{c.security.description}</p>
                <ExternalLink
                  href={`${GITHUB_URL}/blob/main/docs/ARCHITECTURE.md`}
                  className="text-link"
                >
                  {c.security.link}
                  <ArrowUpRight size={15} />
                </ExternalLink>
              </div>
              <div className="vault-illustration">
                <div className="vault-caption">
                  <span>
                    <Folder size={15} />
                    {c.security.treeTitle}
                  </span>
                  <LockKeyhole size={14} />
                </div>
                <div className="file-tree">
                  <div className="tree-root">
                    <Folder size={16} />
                    {c.security.treeRoot}
                  </div>
                  <div className="tree-branch">
                    <div>
                      <Folder size={15} />
                      {c.security.treeFolder}
                    </div>
                    <div className="tree-entries">
                      {c.security.treeFiles.map((name) => (
                        <div key={name}>
                          <FileLock2 size={15} />
                          {name}
                          <span className="file-dot" />
                        </div>
                      ))}
                    </div>
                    <div>
                      <FileKey2 size={15} />
                      {c.security.treeRecipient}
                    </div>
                  </div>
                </div>
                <span className="encrypted-badge">
                  <ShieldCheck size={12} />
                  {c.security.treeBadge}
                </span>
                <div className="private-key">
                  <KeyRound size={20} />
                  <div>
                    <b>{c.security.keyTitle}</b>
                    <span>{c.security.keyBody}</span>
                  </div>
                  <Check size={14} />
                </div>
                <p className="tree-footnote">{c.security.footnote}</p>
              </div>
              <div className="security-points">
                {c.security.points.map((point) => (
                  <article key={point.title}>
                    <h3>
                      <span />
                      {point.title}
                    </h3>
                    <p>{point.body}</p>
                  </article>
                ))}
              </div>
            </div>
          </section>
          <section
            id="guide"
            className="guide-section section-shell"
            aria-labelledby="guide-title"
          >
            <div className="section-heading">
              <span className="eyebrow">{c.guide.eyebrow}</span>
              <h2 id="guide-title">{c.guide.title}</h2>
            </div>
            <ol className="guide-steps">
              {c.guide.items.map((step, index) => (
                <li key={step.title}>
                  <span className="step-number">
                    {String(index + 1).padStart(2, "0")}
                  </span>
                  <h3>{step.title}</h3>
                  <p>{step.body}</p>
                </li>
              ))}
            </ol>
            <ExternalLink href={DOCS_URL} className="text-link">
              {c.guide.link}
              <ArrowUpRight size={15} />
            </ExternalLink>
          </section>
          <DownloadSection
            c={c}
            device={device}
            releaseState={releaseState}
            mobile={
              /Android|iPhone|iPad|iPod/i.test(navigator.userAgent) ||
              (/Mac/.test(navigator.platform) && navigator.maxTouchPoints > 1)
            }
          />
          <section
            id="faq"
            className="faq-section section-shell"
            aria-labelledby="faq-title"
          >
            <h2 id="faq-title">{c.faq.title}</h2>
            <div className="faq-list">
              {c.faq.items.map((item) => (
                <details key={item.question}>
                  <summary>
                    {item.question}
                    <ChevronDown size={17} />
                  </summary>
                  <p>{item.answer}</p>
                </details>
              ))}
            </div>
          </section>
        </main>
        <footer className="site-footer section-shell">
          <div className="footer-main">
            <div>
              <a href="#" className="brand">
                <img src="/icon.svg" width="36" height="36" alt="" />
                <span>{c.brand}</span>
              </a>
              <p>{c.footer.tagline}</p>
            </div>
            <div className="footer-links">
              <ExternalLink href={GITHUB_URL}>{c.footer.source}</ExternalLink>
              <ExternalLink href={RELEASES_URL}>
                {c.footer.releases}
              </ExternalLink>
              <ExternalLink href={`${GITHUB_URL}/issues`}>
                {c.footer.issues}
              </ExternalLink>
              <ExternalLink href={`${GITHUB_URL}#readme`}>
                {c.footer.docs}
              </ExternalLink>
            </div>
          </div>
          <div className="footer-bottom">
            <span>{c.brand}</span>
            <span>
              <Sprout size={13} />
              {c.footer.note}
            </span>
            <a href="#download">
              {c.nav.download}
              <ArrowDown size={12} />
            </a>
          </div>
        </footer>
      </MotionConfig>
    </LazyMotion>
  );
}
