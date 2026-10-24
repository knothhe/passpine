import { useEffect, useState } from "react";
import {
  ArrowUp,
  ArrowUpRight,
  EyeOff,
  HardDrive,
  Mail,
  ServerOff,
  Wifi,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { ExternalLink, SiteLayout } from "../components/site-chrome";
import type { Copy, PolicyBlock } from "../constants/copy";
import {
  privacyPolicy,
  privacySectionIds,
  type PrivacySectionId,
} from "../constants/routes";
import { GITHUB_URL } from "../lib/downloads";
import { useLanguage, type LanguageControl } from "../lib/language";

const summaryIcons = [ServerOff, EyeOff, HardDrive, Wifi];
const POLICY_SOURCE_URL = `${GITHUB_URL}/blob/main/website/src/constants/copy.ts`;

export function formatPolicyDate(iso: string, language: "en" | "zh-CN") {
  const [year, month, day] = iso.split("-").map(Number);
  // Format the calendar date itself, independent of the visitor's time zone.
  return new Intl.DateTimeFormat(language, {
    dateStyle: "long",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(year, month - 1, day)));
}

// Highlights the last section whose heading has passed the reading line; it
// runs only in the browser.
function useActiveSection(ids: readonly PrivacySectionId[]) {
  const [active, setActive] = useState<PrivacySectionId>(ids[0]);
  useEffect(() => {
    let frame = 0;
    const update = () => {
      frame = 0;
      const line = window.innerHeight * 0.3;
      const atEnd =
        window.innerHeight + window.scrollY >=
        document.documentElement.scrollHeight - 2;
      const passed = ids.filter(
        (id) => (document.getElementById(id)?.getBoundingClientRect().top ?? 1) <= line,
      );
      setActive(atEnd ? ids[ids.length - 1] : (passed.at(-1) ?? ids[0]));
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
    };
  }, [ids]);
  return active;
}

function Block({ block }: { block: PolicyBlock }) {
  if (block.kind === "paragraph") return <p>{block.text}</p>;
  if (block.kind === "list")
    return (
      <ul className="policy-list">
        {block.items.map((item) =>
          typeof item === "string" ? (
            <li key={item}>{item}</li>
          ) : (
            <li key={item.term}>
              <strong>{item.term}</strong>
              {item.text}
            </li>
          ),
        )}
      </ul>
    );
  return (
    <div className="policy-table-wrap">
      <table className="policy-table">
        <thead>
          <tr>
            <th scope="col">{block.head[0]}</th>
            <th scope="col">{block.head[1]}</th>
          </tr>
        </thead>
        <tbody>
          {block.rows.map((row) => (
            <tr key={row.name}>
              <th scope="row">
                <code>{row.name}</code>
              </th>
              <td>{row.text}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function PrivacyPage({
  c,
  control,
  language,
}: {
  c: Copy;
  control: LanguageControl;
  language: "en" | "zh-CN";
}) {
  const p = c.privacy;
  const active = useActiveSection(privacySectionIds);
  return (
    <SiteLayout c={c} control={control}>
      <article className="policy" aria-labelledby="policy-title">
        <header className="policy-hero section-shell">
          <span className="eyebrow">{p.eyebrow}</span>
          <h1 id="policy-title">{p.title}</h1>
          <p className="policy-intro">{p.intro}</p>
          <p className="policy-meta">
            <time dateTime={privacyPolicy.updated}>
              {p.updated(formatPolicyDate(privacyPolicy.updated, language))}
            </time>
            <span aria-hidden="true">·</span>
            <span>{p.reviewed(privacyPolicy.reviewedVersion)}</span>
            <span aria-hidden="true">·</span>
            <ExternalLink href={POLICY_SOURCE_URL} className="text-link">
              {p.source}
              <ArrowUpRight size={12} />
            </ExternalLink>
          </p>
        </header>
        <section
          className="policy-summary section-shell"
          aria-labelledby="policy-summary-title"
        >
          <h2 id="policy-summary-title" className="eyebrow">
            {p.summaryTitle}
          </h2>
          <ul>
            {p.summary.map((item, index) => {
              const Icon = summaryIcons[index];
              return (
                <li key={item.title}>
                  <Icon size={20} strokeWidth={1.5} aria-hidden="true" />
                  <h3>{item.title}</h3>
                  <p>{item.body}</p>
                </li>
              );
            })}
          </ul>
        </section>
        <div className="policy-body section-shell">
          <nav className="policy-toc" aria-labelledby="policy-toc-title">
            <h2 id="policy-toc-title" className="eyebrow">
              {p.tocLabel}
            </h2>
            <ol>
              {privacySectionIds.map((id, index) => (
                <li key={id}>
                  <a
                    href={`#${id}`}
                    aria-current={active === id ? "location" : undefined}
                  >
                    <span aria-hidden="true">
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    {p.sections[id].title}
                  </a>
                </li>
              ))}
            </ol>
          </nav>
          <div className="policy-content">
            {privacySectionIds.map((id, index) => {
              const section = p.sections[id];
              return (
                <section key={id} id={id} aria-labelledby={`${id}-title`}>
                  <h2 id={`${id}-title`}>
                    <span aria-hidden="true">
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    {section.title}
                  </h2>
                  {section.blocks.map((block, blockIndex) => (
                    <Block key={blockIndex} block={block} />
                  ))}
                  {id === "contact" && (
                    <div className="policy-contact">
                      <Button variant="outline" size="sm" asChild>
                        <a href={`mailto:${privacyPolicy.contactEmail}`}>
                          <Mail />
                          {p.contactEmail(privacyPolicy.contactEmail)}
                        </a>
                      </Button>
                      <Button variant="outline" size="sm" asChild>
                        <a
                          href={`${GITHUB_URL}/issues`}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {p.contactLink}
                          <ArrowUpRight />
                        </a>
                      </Button>
                    </div>
                  )}
                </section>
              );
            })}
            <a href="#main" className="text-link policy-top">
              {p.backToTop}
              <ArrowUp size={13} />
            </a>
          </div>
        </div>
      </article>
    </SiteLayout>
  );
}

export function PrivacyApp() {
  const { copy, language, control } = useLanguage("privacy");
  return <PrivacyPage c={copy} control={control} language={language} />;
}
