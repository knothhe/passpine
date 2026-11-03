import { useEffect, useState } from 'react';
import { ArrowDownUp, ChevronDown, ChevronUp, Crosshair, Grid2X2, Ruler, X } from 'lucide-react';
import { useCopy } from './LanguageProvider';
import { AuditOverlay, useLayoutAudit, type Insets, type SelectionSlot } from '../layout-audit';
import './DevelopmentAudit.css';

const portalSelector = '[data-radix-popper-content-wrapper], [role="dialog"], [role="alertdialog"], [role="menu"]';

/** Mounted only by development entrypoints, alongside the unchanged product tree. */
export default function DevelopmentAudit({ surface }: { surface: 'popup' | 'options' }) {
  const c = useCopy().audit;
  const [root] = useState(() => document.getElementById('root'));
  const [open, setOpen] = useState(false), [collapsed, setCollapsed] = useState(false);
  const [inspect, setInspect] = useState(false), [grid, setGrid] = useState(false);
  const [slot, setSlot] = useState<SelectionSlot>('selected');
  const [dock, setDock] = useState<'top' | 'bottom'>('bottom');
  const audit = useLayoutAudit({ root, enabled: open, inspect: open && inspect, pickSlot: slot, portalSelector });

  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if (event.isComposing || event.repeat || event.getModifierState('AltGraph')) return;
      const toggle = event.code === 'KeyL' && event.altKey && event.shiftKey && event.ctrlKey !== event.metaKey;
      if (!toggle && !(open && event.key === 'Escape')) return;
      event.preventDefault(); event.stopImmediatePropagation();
      if (toggle) setOpen(value => !value);
      else { setOpen(false); setInspect(false); }
    };
    document.addEventListener('keydown', shortcut, true);
    return () => document.removeEventListener('keydown', shortcut, true);
  }, [open]);

  const choose = (next: SelectionSlot) => { setSlot(next); setInspect(true); setCollapsed(true); };
  const metric = (label: string, value: string) => <div><dt>{label}</dt><dd>{value}</dd></div>;
  const insets = (values: Insets) => <dl className="development-audit-insets">{([['top', c.top], ['right', c.right], ['bottom', c.bottom], ['left', c.left]] as const).map(([key, label]) => <div key={key}><dt>{label}</dt><dd>{c.pixels(values[key])}</dd></div>)}</dl>;
  const selected = slot === 'reference' ? audit.reference : audit.selected;
  const selectedIndex = audit.elements.findIndex(item => item.element === selected?.element);

  if (!open) return surface === 'popup' ? null : <div data-layout-audit-ui className="development-audit-launcher" data-surface={surface}>
    <button type="button" aria-label={c.devOpen} title={c.devShortcut} onClick={() => setOpen(true)}><Ruler size={16}/></button>
  </div>;

  return <>
    <AuditOverlay measurement={audit.selected} reference={audit.reference} comparison={audit.comparison} rootRect={audit.rootRect} grid={grid ? 8 : false} dimensions alignment padding margin formatValue={c.pixels}/>
    <aside data-layout-audit-ui className="development-audit" data-surface={surface} data-dock={dock} aria-label={c.devTitle} onPointerDown={event => event.stopPropagation()}>
      <header>
        <Ruler size={14}/><strong>{c.devTitle}</strong>
        <button type="button" aria-pressed={inspect} title={c.inspect} aria-label={c.inspect} onClick={() => setInspect(value => !value)}><Crosshair size={15}/></button>
        <button type="button" aria-label={dock === 'bottom' ? c.devMoveTop : c.devMoveBottom} onClick={() => setDock(value => value === 'bottom' ? 'top' : 'bottom')}><ArrowDownUp size={15}/></button>
        <button type="button" aria-label={collapsed ? c.devExpand : c.devCollapse} onClick={() => setCollapsed(value => !value)}>{collapsed ? <ChevronUp size={15}/> : <ChevronDown size={15}/>}</button>
        <button type="button" aria-label={c.devClose} onClick={() => { setOpen(false); setInspect(false); }}><X size={15}/></button>
      </header>
      {!collapsed && <div className="development-audit-body">
        <p className="development-audit-help">{c.devHelp}</p>
        <div className="development-audit-tools">
          <button type="button" aria-pressed={inspect && slot === 'selected'} onClick={() => choose('selected')}>{c.pickPrimary}</button>
          <button type="button" aria-pressed={inspect && slot === 'reference'} onClick={() => choose('reference')}>{c.pickReference}</button>
          <button type="button" aria-label={c.grid} title={c.grid} aria-pressed={grid} onClick={() => setGrid(value => !value)}><Grid2X2 size={14}/></button>
        </div>
        <label className="development-audit-tree">{c.anatomy}<select aria-label={c.selection} value={selectedIndex} onChange={event => {
          const item = audit.elements[Number(event.target.value)];
          if (item) audit.select(item.element, slot);
        }}><option value={-1}>{c.noSelection}</option>{audit.elements.map((item, index) => <option key={index} value={index}>{`${'· '.repeat(Math.min(item.depth, 5))}${item.selector}`}</option>)}</select></label>
        {selected ? <>
          <code className="development-audit-selector">{selected.selector}</code>
          {selected.clipped && <p className="development-audit-clipped">{c.clipped}</p>}
          <dl className="development-audit-metrics">
            {metric(c.width, c.pixels(selected.rect.width))}{metric(c.height, c.pixels(selected.rect.height))}
            {metric(c.fontSize, c.pixels(selected.font.size))}{metric(c.lineHeight, selected.font.lineHeight)}
            {metric(c.fontWeight, selected.font.weight)}{metric(c.inset, c.pixels(selected.localRect.x))}
          </dl>
          <h3>{c.padding}</h3>{insets(selected.padding)}
          <h3>{c.margin}</h3>{insets(selected.margin)}
        </> : <p className="development-audit-help">{c.noSelection}</p>}
        {audit.reference && <section className="development-audit-comparison"><h3>{c.reference}</h3><code>{audit.reference.selector}</code>{audit.comparison && <dl className="development-audit-metrics">{metric(c.horizontalGap, c.pixels(audit.comparison.horizontalGap))}{metric(c.verticalGap, c.pixels(audit.comparison.verticalGap))}</dl>}<button type="button" onClick={audit.clearReference}>{c.clearReference}</button></section>}
        <footer><button type="button" onClick={audit.clear}>{c.clearSelection}</button><small>{c.devShortcut}</small></footer>
      </div>}
    </aside>
  </>;
}
