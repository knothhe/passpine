import { ArrowLeft, ArrowRight } from "lucide-react";
import { Button } from "../components/ui/button";
import { SiteLayout } from "../components/site-chrome";
import type { Copy } from "../constants/copy";
import { routes } from "../constants/routes";
import { useLanguage, type LanguageControl } from "../lib/language";

export function NotFoundPage({
  c,
  control,
}: {
  c: Copy;
  control: LanguageControl;
}) {
  return (
    <SiteLayout c={c} control={control}>
      <section className="not-found section-shell" aria-labelledby="nf-title">
        <span className="eyebrow">{c.notFound.eyebrow}</span>
        <h1 id="nf-title">{c.notFound.title}</h1>
        <p>{c.notFound.description}</p>
        <div className="hero-actions">
          <Button asChild>
            <a href={routes.home}>
              <ArrowLeft />
              {c.notFound.home}
            </a>
          </Button>
          <a href={routes.privacy} className="text-link">
            {c.notFound.privacy}
            <ArrowRight size={15} />
          </a>
        </div>
      </section>
    </SiteLayout>
  );
}

export function NotFoundApp() {
  const { copy, control } = useLanguage("notFound");
  return <NotFoundPage c={copy} control={control} />;
}
