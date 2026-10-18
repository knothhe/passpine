import { useState } from 'react';
import { Callout, Heading, Select, Text } from '@radix-ui/themes';
import { Languages } from 'lucide-react';
import type { LanguagePreference } from '../lib/i18n';
import { errorMessage } from '../lib/errors';
import { useCopy, useLanguage } from './LanguageProvider';

export function LanguageSettings() {
  const copy = useCopy();
  const { language, loading, saving, loadFailed, setLanguage } = useLanguage();
  const [error, setError] = useState<unknown>();
  return <>
    <div className="options-section-heading"><Languages size={18}/><Heading as="h2" size="3">{copy.language.title}</Heading></div>
    <div className="appearance-mode-row"><Text as="p" size="2" color="gray">{copy.language.description}</Text>
      <Select.Root value={language} disabled={loading || saving} onValueChange={value => { setError(undefined); void setLanguage(value as LanguagePreference).catch(setError); }}>
        <Select.Trigger aria-label={copy.language.title} aria-busy={loading || saving || undefined}/><Select.Content>
          <Select.Item value="system">{copy.language.system}</Select.Item><Select.Item value="en">{copy.language.english}</Select.Item><Select.Item value="zh-CN">{copy.language.chinese}</Select.Item>
        </Select.Content>
      </Select.Root>
    </div>
    {(error || loadFailed) && <Callout.Root mt="3" size="1" color="red" role="alert"><Callout.Text>{error ? errorMessage(error, copy) : copy.language.loadFailed}</Callout.Text></Callout.Root>}
  </>;
}
