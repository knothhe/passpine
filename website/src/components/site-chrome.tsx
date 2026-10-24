import type { ReactNode } from "react";
import { LazyMotion, domAnimation, MotionConfig } from "motion/react";
import { ArrowDown, ArrowUpRight, GitBranch, Globe2, Sprout } from "lucide-react";
import type { Copy, LanguagePreference } from "../constants/copy";
import { routes, sectionHref } from "../constants/routes";
import { GITHUB_URL, RELEASES_URL } from "../lib/downloads";
import type { LanguageControl } from "../lib/language";

export function ExternalLink({
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

const navItems = ["features", "security", "guide", "faq"] as const;

function SiteHeader({ c, control }: { c: Copy; control: LanguageControl }) {
  return (
    <header className="site-header">
      <a href={routes.home} className="brand" aria-label={c.nav.home}>
        <img src="/icon.svg" width="43" height="43" alt="" />
        <span>{c.brand}</span>
      </a>
      <nav aria-label={c.nav.label}>
        {navItems.map((id) => (
          <a key={id} href={sectionHref(id)} data-section={id}>
            {c.nav[id]}
          </a>
        ))}
      </nav>
      <div className="header-actions">
        <div className="language-select">
          <Globe2 size={14} aria-hidden="true" />
          <select
            aria-label={c.language.label}
            value={control.preference}
            onChange={(event) =>
              control.setPreference(event.target.value as LanguagePreference)
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
  );
}

function SiteFooter({ c }: { c: Copy }) {
  return (
    <footer className="site-footer section-shell">
      <div className="footer-main">
        <div>
          <a href={routes.home} className="brand" aria-label={c.nav.home}>
            <img src="/icon.svg" width="36" height="36" alt="" />
            <span>{c.brand}</span>
          </a>
          <p>{c.footer.tagline}</p>
        </div>
        <nav className="footer-links" aria-label={c.footer.label}>
          <ExternalLink href={GITHUB_URL}>{c.footer.source}</ExternalLink>
          <ExternalLink href={RELEASES_URL}>{c.footer.releases}</ExternalLink>
          <ExternalLink href={`${GITHUB_URL}/issues`}>
            {c.footer.issues}
          </ExternalLink>
          <ExternalLink href={`${GITHUB_URL}#readme`}>
            {c.footer.docs}
          </ExternalLink>
          <a href={routes.privacy}>{c.footer.privacy}</a>
        </nav>
      </div>
      <div className="footer-bottom">
        <span>{c.brand}</span>
        <span>
          <Sprout size={13} />
          {c.footer.note}
        </span>
        <a href={sectionHref("download")}>
          {c.nav.download}
          <ArrowDown size={12} />
        </a>
      </div>
    </footer>
  );
}

export function SiteLayout({
  c,
  control,
  children,
}: {
  c: Copy;
  control: LanguageControl;
  children: ReactNode;
}) {
  return (
    <LazyMotion features={domAnimation}>
      <MotionConfig reducedMotion="user">
        <a className="skip-link" href="#main">
          {c.skip}
        </a>
        <SiteHeader c={c} control={control} />
        <main id="main">{children}</main>
        <SiteFooter c={c} />
      </MotionConfig>
    </LazyMotion>
  );
}
