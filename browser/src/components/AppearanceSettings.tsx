import { useState } from 'react';
import { Callout, Heading, RadioCards, SegmentedControl, Text, Theme } from '@radix-ui/themes';
import { Monitor, Moon, Palette, Sun } from 'lucide-react';
import { themeStyles, type Appearance, type AppearancePreferences, type ThemeStyle } from '../lib/appearance';
import { errorMessage } from '../lib/errors';
import { ApplicationIcon } from './Primitives';
import { useAppearance } from './Provider';

export function AppearanceSettings() {
  const { preferences, resolvedAppearance, saving, setPreferences } = useAppearance();
  const [error, setError] = useState('');
  const change = (next: AppearancePreferences) => {
    setError('');
    void setPreferences(next).catch(error => setError(errorMessage(error)));
  };
  return <>
    <div className="options-section-heading"><Palette size={18}/><Heading as="h2" size="3">外观</Heading></div>
    <Text as="p" size="2" color="gray" className="appearance-description">选择主题风格，界面和图标会一起切换。</Text>
    <RadioCards.Root className="style-choices" value={preferences.style} onValueChange={style => change({ ...preferences, style: style as ThemeStyle })} columns={{ initial: '1', xs: '2' }} gap="3" aria-label="主题风格" disabled={saving}>
      {themeStyles.map(style => <RadioCards.Item key={style.id} value={style.id} className="style-choice" aria-label={style.label}>
        <Theme appearance={resolvedAppearance} accentColor={style.id === 'monochrome' ? 'gray' : 'orange'} grayColor={style.id === 'monochrome' ? 'gray' : 'sand'} radius="medium" className="style-preview">
          <div className="style-preview-header"><ApplicationIcon style={style.id} size={28}/><span>Passpine</span><span className="style-preview-action"/></div>
          <div className="style-preview-row"><span/><i/><i/></div><div className="style-preview-row"><span/><i/><i/></div>
        </Theme>
        <div className="style-choice-caption"><Text as="span" size="2" weight="medium">{style.label}</Text><Text as="span" size="1" color="gray">{style.description}</Text></div>
      </RadioCards.Item>)}
    </RadioCards.Root>
    <div className="appearance-mode-row"><div><Text as="div" size="2" weight="medium">明暗模式</Text><Text as="p" size="1" color="gray">跟随系统，或固定使用浅色、深色。</Text></div><SegmentedControl.Root value={preferences.appearance} onValueChange={appearance => change({ ...preferences, appearance: appearance as Appearance })} aria-label="明暗模式" disabled={saving}>
      <SegmentedControl.Item value="system"><Monitor size={14}/>跟随系统</SegmentedControl.Item><SegmentedControl.Item value="light"><Sun size={14}/>浅色</SegmentedControl.Item><SegmentedControl.Item value="dark"><Moon size={14}/>深色</SegmentedControl.Item>
    </SegmentedControl.Root></div>
    <Text as="p" size="1" color="gray" className="appearance-help">设置自动保存，并同步到已打开的扩展页面。客户端外观可在客户端设置中选择。</Text>
    {error && <Callout.Root mt="3" size="1" color="red" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
  </>;
}
