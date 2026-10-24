import type { ReactNode } from 'react';

// Adapted from Readomi's options settings-section: one surface, divided rows,
// optional captions and a consistent place for each row's control.
export function SettingsGroup({ caption, children, className = '' }: { caption?: ReactNode; children: ReactNode; className?: string }) {
  return <div className={`settings-group-wrap ${className}`}>
    {caption && <h2 className="settings-group-caption">{caption}</h2>}
    <div className="settings-group">{children}</div>
  </div>;
}

export function SettingsRow({ label, description, control, children, className = '' }: { label: ReactNode; description?: ReactNode; control?: ReactNode; children?: ReactNode; className?: string }) {
  return <div className={`settings-row ${className}`}>
    <div className="settings-row-heading"><div className="settings-row-label">{label}{description && <p>{description}</p>}</div>{control && <div className="settings-row-control">{control}</div>}</div>
    {children}
  </div>;
}
