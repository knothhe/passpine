import { useCopy, useLanguage } from './LanguageProvider';
import { useEffect, useState } from 'react';
import { Callout, Heading, RadioCards, SegmentedControl, Text, Theme } from '@radix-ui/themes';
import { Monitor, Moon, Palette, Sun } from 'lucide-react';
import { fontScales, themeStyles, type Appearance, type AppearancePreferences, type FontScale, type ThemeStyle } from '../lib/appearance';
import { errorMessage } from '../lib/errors';
import { ApplicationIcon } from './Primitives';
import { useAppearance } from './Provider';

export function AppearanceSettings() {
  const copy = useCopy();
  const { locale } = useLanguage();
  const { preferences, resolvedAppearance, saving, setPreferences } = useAppearance();
  const [error, setError] = useState('');
  useEffect(() => { setError(''); }, [locale]);
  const change = (next: AppearancePreferences) => {
    setError('');
    void setPreferences(next).catch(error => setError(errorMessage(error, copy)));
  };
  return <>
    <div className="options-section-heading"><Palette size={18}/><Heading as="h2" size="3">{copy.appearance.title}</Heading></div>
    <Text as="p" size="2" color="gray" className="appearance-description">{copy.appearance.description}</Text>
    <RadioCards.Root className="style-choices" value={preferences.style} onValueChange={style => change({ ...preferences, style: style as ThemeStyle })} columns={{ initial: '1', xs: '2' }} gap="3" aria-label={copy.appearance.themeStyle} disabled={saving}>
      {themeStyles.map(style => <RadioCards.Item key={style.id} value={style.id} className="style-choice" aria-label={copy.appearance[style.id].label}>
        <Theme appearance={resolvedAppearance} accentColor={style.id === 'monochrome' ? 'gray' : 'orange'} grayColor={style.id === 'monochrome' ? 'gray' : 'sand'} radius="medium" className="style-preview">
          <div className="style-preview-header"><ApplicationIcon style={style.id} size={28}/><span>{copy.brand}</span><span className="style-preview-action"/></div>
          <div className="style-preview-row"><span/><i/><i/></div><div className="style-preview-row"><span/><i/><i/></div>
        </Theme>
        <div className="style-choice-caption"><Text as="span" size="2" weight="medium">{copy.appearance[style.id].label}</Text><Text as="span" size="1" color="gray">{copy.appearance[style.id].description}</Text></div>
      </RadioCards.Item>)}
    </RadioCards.Root>
    <div className="appearance-mode-row"><div><Text as="div" size="2" weight="medium">{copy.appearance.mode}</Text><Text as="p" size="1" color="gray">{copy.appearance.modeDescription}</Text></div><SegmentedControl.Root size="1" value={preferences.appearance} onValueChange={appearance => change({ ...preferences, appearance: appearance as Appearance })} aria-label={copy.appearance.mode} disabled={saving}>
      <SegmentedControl.Item value="system"><Monitor size={14}/>{copy.appearance.system}</SegmentedControl.Item><SegmentedControl.Item value="light"><Sun size={14}/>{copy.appearance.light}</SegmentedControl.Item><SegmentedControl.Item value="dark"><Moon size={14}/>{copy.appearance.dark}</SegmentedControl.Item>
    </SegmentedControl.Root></div>
    <div className="appearance-mode-row font-scale-row"><div><Text as="div" size="2" weight="medium">{copy.appearance.fontSize}</Text><Text as="p" size="1" color="gray">{copy.appearance.fontSizeDescription}</Text></div><SegmentedControl.Root size="1" value={String(preferences.fontScale)} onValueChange={value => change({ ...preferences, fontScale: Number(value) as FontScale })} aria-label={copy.appearance.fontSize} disabled={saving}>
      {fontScales.map(scale => <SegmentedControl.Item key={scale} value={String(scale)}>{copy.common.percent(scale)}</SegmentedControl.Item>)}
    </SegmentedControl.Root></div>
    <Text as="p" size="1" color="gray" className="appearance-help">{copy.appearance.help}</Text>
    {error && <Callout.Root mt="3" size="1" color="red" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
  </>;
}
