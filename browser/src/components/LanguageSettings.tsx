import { useState } from 'react';
import { Callout, Select } from '@radix-ui/themes';
import { SettingsRow } from './SettingsLayout';
import type { LanguagePreference } from '../lib/i18n';
import { errorMessage } from '../lib/errors';
import { useCopy, useLanguage } from './LanguageProvider';

export function LanguageSettings() {
  const copy = useCopy();
  const { language, loading, saving, loadFailed, setLanguage } = useLanguage();
  const [error, setError] = useState<unknown>();
  return <SettingsRow label={copy.language.title} control={<Select.Root value={language} disabled={loading || saving} onValueChange={value => { setError(undefined); void setLanguage(value as LanguagePreference).catch(setError); }}>
        <Select.Trigger aria-label={copy.language.title} aria-busy={loading || saving || undefined}/><Select.Content>
          <Select.Item value="system">{copy.language.system}</Select.Item><Select.Item value="en">{copy.language.english}</Select.Item><Select.Item value="zh-CN">{copy.language.chinese}</Select.Item>
        </Select.Content>
      </Select.Root>}>
    {(error || loadFailed) && <Callout.Root mt="3" size="1" color="red" role="alert"><Callout.Text>{error ? errorMessage(error, copy) : copy.language.loadFailed}</Callout.Text></Callout.Root>}
  </SettingsRow>;
}
