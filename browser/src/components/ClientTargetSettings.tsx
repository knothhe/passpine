import { useEffect, useRef, useState } from 'react';
import { Callout, SegmentedControl } from '@radix-ui/themes';
import { SettingsGroup, SettingsRow } from './SettingsLayout';
import { canSelectClientTarget, loadClientTarget, saveClientTarget, subscribeClientTarget, type ClientTarget } from '../lib/client-target';
import { useCopy } from './LanguageProvider';

export function useClientTarget(demo = false) {
  const [target, setTarget] = useState<ClientTarget>('release');
  const [connectionRevision, setConnectionRevision] = useState(0);
  const [canSelect, setCanSelect] = useState(false), [loading, setLoading] = useState(true), [saving, setSaving] = useState(false);
  const [error, setError] = useState<'load' | 'save'>();
  const revision = useRef(0), active = useRef(false), pendingSave = useRef(false);
  useEffect(() => {
    active.current = true; setLoading(true); setCanSelect(false); setError(undefined);
    const unsubscribe = subscribeClientTarget(next => {
      revision.current++;
      if (active.current) { setTarget(next); setConnectionRevision(value => value + 1); setLoading(false); setError(undefined); }
    }, demo);
    const initial = revision.current;
    void canSelectClientTarget(demo).then(allowed => { if (active.current) setCanSelect(allowed); });
    void loadClientTarget(demo).then(next => {
      if (active.current && revision.current === initial) { setTarget(next); setLoading(false); }
    }).catch(() => {
      if (active.current && revision.current === initial) { setTarget('release'); setLoading(false); setError('load'); }
    });
    return () => { active.current = false; revision.current++; unsubscribe(); };
  }, [demo]);
  const changeTarget = async (next: ClientTarget) => {
    if (pendingSave.current || loading || !canSelect || next === target) return;
    const request = revision.current;
    pendingSave.current = true; setSaving(true); setError(undefined);
    try {
      const saved = await saveClientTarget(next, demo);
      // A storage notification may have already applied this write or a newer
      // selection from another options page. Never replace that newer value.
      if (active.current && revision.current === request) { revision.current++; setTarget(saved); }
    } catch { if (active.current) setError('save'); }
    finally { pendingSave.current = false; if (active.current) setSaving(false); }
  };
  return { target, connectionRevision, canSelect, loading, saving, error, changeTarget };
}

export function ClientTargetSettings({ state }: { state: ReturnType<typeof useClientTarget> }) {
  const copy = useCopy();
  return <SettingsGroup><SettingsRow label={copy.clientTarget.title} description={state.canSelect ? undefined : copy.clientTarget.storeDescription} control={state.canSelect && <SegmentedControl.Root size="1" value={state.target} disabled={state.loading || state.saving} aria-label={copy.clientTarget.title} aria-busy={state.loading || state.saving || undefined} onValueChange={target => { void state.changeTarget(target as ClientTarget); }}>
        <SegmentedControl.Item value="release">{copy.clientTarget.release}</SegmentedControl.Item><SegmentedControl.Item value="dev">{copy.clientTarget.dev}</SegmentedControl.Item>
      </SegmentedControl.Root>}>
    {state.error && <Callout.Root mt="3" size="1" color="red" role="alert"><Callout.Text>{state.error === 'load' ? copy.clientTarget.loadFailed : copy.clientTarget.saveFailed}</Callout.Text></Callout.Root>}
  </SettingsRow></SettingsGroup>;
}
