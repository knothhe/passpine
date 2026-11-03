// Passpine-specific shortcuts are adapters, not part of the reusable inspector.
export const popupTargets = {
  shell: '.popup-shell', site: '.site-context', search: '.search-section .rt-TextFieldRoot',
  row: '.entry-row', title: '.entry-text>span:last-child', path: '.entry-path>.rt-Text',
  action: '.account-actions .account-action', footer: '.app-footer', field: '.credential-field',
  label: '.credential-label', editor: '.editor-fields>.field-label', generator: '.generator-screen',
  notice: '.notice', dialog: '[role="dialog"], [role="alertdialog"]',
} as const;
export const optionsTargets = {
  shell: '.options-shell', navigation: '.options-navigation', heading: '.options-title h1',
  group: '.settings-group', setting: '.settings-row', settingLabel: '.settings-row-label',
  control: '.settings-row-control', dialog: '[role="dialog"], [role="alertdialog"]',
} as const;
export type Preset = keyof typeof popupTargets | keyof typeof optionsTargets;
export const portalSelector = '[data-radix-popper-content-wrapper], .rt-DialogContent, .rt-AlertDialogContent, .rt-DialogOverlay, .rt-AlertDialogOverlay';
export const scenarios = ['default', 'rich', 'long', 'empty', 'unconfigured', 'fill-failure', 'association-failure'] as const;
export type Scenario = typeof scenarios[number];
export type Surface = 'popup' | 'options';
