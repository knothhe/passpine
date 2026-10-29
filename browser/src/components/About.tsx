import { Heading, Text } from '@radix-ui/themes';
import { ExternalLink } from 'lucide-react';
import { version } from '../../package.json';
import { useCopy } from './LanguageProvider';
import { ApplicationIcon } from './Primitives';

export function About({ extensionId, firefox = false }: { extensionId: string; firefox?: boolean }) {
  const copy = useCopy().about;
  return <section className="about-page">
    <div className="options-title"><Heading as="h1" size="6" tabIndex={-1}>{copy.title}</Heading></div>
    <div className="about-hero"><div className="about-brand"><ApplicationIcon size={48}/><Heading as="h2" size="5">{copy.name}</Heading></div><Text as="p" size="2" className="about-version">{copy.version(version)}</Text><Text as="p" size="2" color="gray">{copy.projectDescription}</Text><a href="https://github.com/knothhe/passpine" target="_blank" rel="noreferrer">{copy.project}<ExternalLink size={13} aria-hidden="true"/></a></div>
    <dl className="diagnostic-list about-information"><div><dt>{copy.browser}</dt><dd>{firefox ? copy.firefox : copy.chromium}</dd></div><div><dt>{copy.extensionID}</dt><dd><code>{extensionId}</code></dd></div></dl>
  </section>;
}
