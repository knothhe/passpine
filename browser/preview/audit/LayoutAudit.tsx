import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, ArrowUpRight, Check, Download, Grid2X2, MousePointer2, Ruler, ScanLine, Sun, Moon } from 'lucide-react';
import { useCopy, useLanguage } from '../../src/components/LanguageProvider';
import { useAppearance } from '../../src/components/Provider';
import { fontScales, type FontScale } from '../../src/lib/appearance';
import type { LanguagePreference } from '../../src/lib/i18n';
import { AuditOverlay, useLayoutAudit, type Measurement, type SelectionSlot } from '../../src/layout-audit';
import { optionsTargets, popupTargets, portalSelector, scenarios, type Preset, type Scenario, type Surface } from './profiles';
import './audit.css';

const findingIDs = ['alignment', 'readability', 'targets'] as const;
const round = (value: number) => Math.round(value * 10) / 10;
const serializable = (value: Measurement | null) => value ? {
  selector: value.selector, rect: value.rect, localRect: value.localRect, visibleRect: value.visibleRect,
  padding: value.padding, margin: value.margin, border: value.border, gap: value.gap, font: value.font,
  scale: value.scale, clipped: value.clipped,
} : null;

export function LayoutAudit() {
  const copy = useCopy(), c = copy.audit, language = useLanguage();
  const { preferences, resolvedAppearance, setPreferences } = useAppearance();
  const [surface, setSurface] = useState<Surface>(() => new URLSearchParams(location.search).get('surface') === 'options' ? 'options' : 'popup');
  const [scenario, setScenario] = useState<Scenario>(() => {
    const value = new URLSearchParams(location.search).get('fixture');
    return scenarios.includes(value as Scenario) ? value as Scenario : 'default';
  });
  const [connection, setConnection] = useState(() => {
    const value = new URLSearchParams(location.search).get('state');
    return value === 'locked' || value === 'disconnected' ? value : 'connected';
  });
  const [viewport, setViewport] = useState(1100);
  const [guides, setGuides] = useState(true), [grid, setGrid] = useState(false), [boxModel, setBoxModel] = useState(true);
  const [inspect, setInspect] = useState(false), [slot, setSlot] = useState<SelectionSlot>('selected');
  const [error, setError] = useState(false), [frameError, setFrameError] = useState(false), [filter, setFilter] = useState('');
  const [root, setRoot] = useState<HTMLElement | null>(null);
  const iframe = useRef<HTMLIFrameElement>(null), disposeFrame = useRef<() => void>(() => {});
  const width = surface === 'popup' ? 380 : viewport, height = surface === 'popup' ? 460 : 720;
  const frameSource = useMemo(() => {
    const params = new URLSearchParams({ view: surface === 'popup' ? 'popup' : 'options', state: connection });
    if (scenario !== 'default') params.set('fixture', scenario);
    return `/?${params}`;
  }, [surface, connection, scenario]);
  const audit = useLayoutAudit({ root, enabled: true, inspect, pickSlot: slot, portalSelector });
  const value = audit.selected;
  const profile = surface === 'popup' ? popupTargets : optionsTargets;
  const entries = Object.entries(profile) as [Preset, string][];
  const nodeFor = (selector: string) => audit.elements.find(node => node.element.matches(selector))?.element;
  const preset = entries.find(([, selector]) => value?.element.matches(selector))?.[0];
  const presetLabel = (key: Preset) => key === 'shell' && surface === 'options' ? c.surfaces.options : c.targets[key];
  const note = surface === 'popup' && preset && preset in c.notes ? c.notes[preset as keyof typeof c.notes] : c.notes.custom;
  const filteredNodes = audit.elements.filter(node => node.selector.toLowerCase().includes(filter.toLowerCase()));

  const bindFrame = useCallback(() => {
    disposeFrame.current();
    setFrameError(false);
    try {
      const doc = iframe.current?.contentDocument;
      if (!doc) { setFrameError(true); return; }
      const sync = () => setRoot(doc.querySelector<HTMLElement>(surface === 'popup' ? '.popup-shell' : '.options-shell'));
      const observer = new MutationObserver(sync);
      observer.observe(doc.documentElement, { subtree: true, childList: true });
      disposeFrame.current = () => observer.disconnect();
      sync();
    } catch { setFrameError(true); }
  }, [surface]);
  useEffect(() => { setRoot(null); disposeFrame.current(); return () => disposeFrame.current(); }, [frameSource]);
  useEffect(() => {
    if (root) audit.select(root);
  }, [root]);
  const save = (promise: Promise<void>) => { setError(false); void promise.catch(() => setError(true)); };
  const pick = (next: SelectionSlot) => { setSlot(next); setInspect(true); };
  const metric = (label: string, result: string) => <div className="audit-metric"><dt>{label}</dt><dd>{result}</dd></div>;
  const insets = (sides: Measurement['padding']) => <div className="audit-padding">{(['top', 'right', 'bottom', 'left'] as const).map(side => <span key={side}><small>{c[side]}</small><b>{c.pixels(round(sides[side]))}</b></span>)}</div>;
  const exportReport = () => {
    const inspected = root?.ownerDocument;
    const report = { schema: 'layout-audit/v1', surface, scenario, initialConnection: connection, section: inspected?.location.hash, viewport: { width, height }, locale: inspected?.documentElement.lang, fontScale: Number(inspected?.documentElement.dataset.fontScale), selected: serializable(audit.selected), reference: serializable(audit.reference), comparison: audit.comparison };
    const url = URL.createObjectURL(new Blob([JSON.stringify(report, null, 2)], { type: 'application/json' }));
    const link = document.createElement('a'); link.href = url; link.download = 'passpine-layout-audit.json'; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  return <main className="audit-page" data-layout-audit-ui>
    <header className="audit-header"><a className="audit-brand" href="/" aria-label={c.back}><img src="/icon48-monochrome.png" alt=""/><strong>{copy.brand}</strong><span>{c.eyebrow}</span></a><a className="audit-back" href="/"><ArrowLeft size={14}/>{c.back}</a></header>
    <section className="audit-intro"><div><p className="audit-kicker">{c.sample}</p><h1>{c.title}</h1><p>{c.intro}</p></div><span className="audit-live"><i/>{c.live}</span></section>
    <div className="audit-scenarios">
      <label>{c.surface}<select value={surface} onChange={event => setSurface(event.target.value as Surface)}>{(['popup', 'options'] as const).map(id => <option key={id} value={id}>{c.surfaces[id]}</option>)}</select></label>
      <label>{c.scenario}<select value={scenario} onChange={event => setScenario(event.target.value as Scenario)}>{scenarios.map(id => <option key={id} value={id}>{c.scenarios[id]}</option>)}</select></label>
      <label>{c.connectionState}<select value={connection} onChange={event => setConnection(event.target.value)}>{(['connected', 'locked', 'disconnected'] as const).map(id => <option key={id} value={id}>{copy.connection.status[id]}</option>)}</select></label>
      {surface === 'options' && <label>{c.viewport}<select value={viewport} onChange={event => setViewport(Number(event.target.value))}>{[390, 760, 1100].map(size => <option key={size} value={size}>{c.pixels(size)}</option>)}</select></label>}
    </div>
    <div className="audit-toolbar" role="group" aria-label={c.tools}>
      <div className="audit-toggles">
        <button type="button" aria-pressed={guides} onClick={() => setGuides(!guides)}><Ruler size={15}/>{c.guides}</button>
        <button type="button" aria-pressed={grid} onClick={() => setGrid(!grid)}><Grid2X2 size={15}/>{c.grid}</button>
        <button type="button" aria-pressed={inspect} onClick={() => setInspect(!inspect)}><MousePointer2 size={15}/>{c.inspect}</button>
        <button type="button" aria-pressed={boxModel} onClick={() => setBoxModel(!boxModel)}>{c.padding}</button>
      </div>
      <div className="audit-preferences">
        <label>{c.fontScale}<select value={preferences.fontScale} onChange={event => save(setPreferences({ ...preferences, fontScale: Number(event.target.value) as FontScale }))}>{fontScales.map(scale => <option key={scale} value={scale}>{c.percentage(scale)}</option>)}</select></label>
        <button type="button" aria-label={c.appearance} title={c.appearance} onClick={() => save(setPreferences({ ...preferences, appearance: resolvedAppearance === 'dark' ? 'light' : 'dark' }))}>{resolvedAppearance === 'dark' ? <Moon size={16}/> : <Sun size={16}/>}</button>
        <select aria-label={c.language} value={language.language} disabled={language.loading || language.saving} onChange={event => save(language.setLanguage(event.target.value as LanguagePreference))}><option value="system">{copy.language.system}</option><option value="zh-CN">{copy.language.chinese}</option><option value="en">{copy.language.english}</option></select>
      </div>
    </div>
    {error && <p className="audit-error" role="alert">{c.saveFailed}</p>}
    <div className={`audit-workspace ${surface === 'options' ? 'audit-workspace-wide' : ''}`}>
      <aside className="audit-anatomy"><div className="audit-panel-heading"><ScanLine size={16}/><h2>{c.presets}</h2></div><p>{c.anatomyHint}</p>
        <nav aria-label={c.presets}>{entries.map(([key, selector], index) => <button type="button" key={key} aria-pressed={preset === key} disabled={!nodeFor(selector)} onClick={() => { const element = nodeFor(selector); if (element) audit.select(element, slot); }}><span>{String(index + 1).padStart(2, '0')}</span>{presetLabel(key)}</button>)}</nav>
        <div className="audit-tree-heading"><h3>{c.fullTree}</h3><span>{c.componentCount(audit.elements.length)}</span></div>
        <input className="audit-filter" aria-label={c.filterComponents} placeholder={c.filterComponents} value={filter} onChange={event => setFilter(event.target.value)}/>
        <div className="audit-tree" role="group" aria-label={c.fullTree}>{filteredNodes.map((node, index) => <button key={index} type="button" title={node.selector} aria-pressed={node.element === value?.element} style={{ paddingLeft: 8 + Math.min(node.depth, 5) * 8 }} onClick={() => audit.select(node.element, slot)}>{node.selector}</button>)}{!filteredNodes.length && <p>{c.emptyTree}</p>}</div>
      </aside>
      <section className="audit-canvas" aria-label={c.surfaces[surface]}>
        <div className="audit-canvas-label"><span>{c.surfaces[surface]}</span><span>{c.dimensions(width, height)}</span></div>
        <div className="audit-compare-toolbar" role="group" aria-label={c.compare}><button type="button" aria-pressed={slot === 'selected' && inspect} onClick={() => pick('selected')}>{c.pickPrimary}</button><button type="button" aria-pressed={slot === 'reference' && inspect} onClick={() => pick('reference')}>{c.pickReference}</button><button type="button" disabled={!audit.reference} onClick={() => { audit.clearReference(); setSlot('selected'); }}>{c.clearReference}</button><button type="button" disabled={!value} onClick={() => audit.selectParent()}>{c.parent}</button></div>
        <div className="audit-stage audit-embedded-stage" style={{ minWidth: width + 48 }}><div className="audit-frame" style={{ width, height }}>
          <iframe ref={iframe} className="audit-embedded" title={c.surfaces[surface]} src={frameSource} width={width} height={height} onLoad={bindFrame}/>
          {!root && <p className="audit-frame-status" role="status">{frameError ? c.unsupportedFrame : c.loadingFrame}</p>}
        </div></div>
        {root && <AuditOverlay ownerDocument={root.ownerDocument} measurement={value} reference={audit.reference} comparison={audit.comparison} rootRect={audit.rootRect} grid={grid ? 8 : false} dimensions={guides} alignment={guides} padding={guides && boxModel} margin={guides && boxModel} formatValue={c.pixels}/>}
        <div className="audit-legend"><span><i className="size"/>{c.sizeLegend}</span><span><i className="align"/>{c.alignLegend}</span>{grid && <span><i className="grid"/>{c.gridLegend}</span>}</div>
        <p className="audit-canvas-help"><MousePointer2 size={14}/>{inspect ? c.inspectHint : c.interactHint}</p>
      </section>
      <aside className="audit-inspector" aria-label={c.selection}>
        <div className="audit-panel-heading"><Ruler size={16}/><h2>{c.measured}</h2></div><h3 className="audit-selected-title">{preset ? presetLabel(preset) : c.customElement}</h3>
        {value ? <>
          <code className="audit-selector">{value.selector}</code>{value.clipped && <p className="audit-clip-note">{c.clipped}</p>}
          <h4>{c.geometry}</h4><dl className="audit-metrics">{metric(c.width, c.pixels(round(value.localRect.width)))}{metric(c.height, c.pixels(round(value.localRect.height)))}{metric(c.inset, c.pixels(round(value.localRect.x)))}{metric(c.rowGap, value.gap.row)}{metric(c.columnGap, value.gap.column)}</dl>
          <h4>{c.padding}</h4>{insets(value.padding)}<h4>{c.margin}</h4>{insets(value.margin)}
          <h4>{c.typography}</h4><dl className="audit-metrics">{metric(c.fontSize, c.pixels(value.font.size))}{metric(c.lineHeight, value.font.lineHeight)}{metric(c.fontWeight, value.font.weight)}{metric(c.letterSpacing, value.font.letterSpacing)}</dl><p className="audit-unit-note">{c.typographyHint}</p>
          <button className="audit-export" type="button" onClick={exportReport}><Download size={14}/>{c.exportReport}</button><p className="audit-unit-note">{c.exportHint}</p>
        </> : <p className="audit-pending" role="status">{root ? c.noSelection : c.loadingFrame}</p>}
        <div className="audit-comparison"><h4>{c.compare}</h4><p>{c.comparisonHint}</p><code>{audit.reference?.selector ?? c.noReference}</code>{audit.comparison && <dl className="audit-metrics">{metric(c.horizontalGap, c.pixels(round(audit.comparison.horizontalGap)))}{metric(c.verticalGap, c.pixels(round(audit.comparison.verticalGap)))}{metric(c.leftAlignment, c.pixels(round(audit.comparison.deltaLeft)))}{metric(c.topAlignment, c.pixels(round(audit.comparison.deltaTop)))}</dl>}</div>
        <div className="audit-observation"><h4>{c.observation}</h4><p>{note.observation}</p><h4>{c.recommendation}</h4><p>{note.recommendation}</p></div>
      </aside>
    </div>
    <section className="audit-findings"><div className="audit-findings-heading"><div><p className="audit-kicker">{c.count(3)}</p><h2>{c.findingsTitle}</h2></div><p>{c.findingsIntro}</p></div><div className="audit-finding-grid">{findingIDs.map((key, index) => <article key={key}><span className="audit-finding-number">{String(index + 1).padStart(2, '0')}</span><h3>{c.findings[key].title}</h3><p>{c.findings[key].body}</p><div><Check size={14}/><span>{c.findings[key].action}</span></div></article>)}</div></section>
    <footer className="audit-page-footer"><p>{c.scaleNote}</p><a href="https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html" target="_blank" rel="noreferrer">{c.source}<ArrowUpRight size={14}/></a></footer>
  </main>;
}
