import { englishCopy as copy } from '../src/i18n/en';

// Resolve page titles from the same catalog in WXT builds and Vite previews.
export function copyHTML() {
  const titles = {
    '%COPY_BRAND%': copy.brand,
    '%COPY_OPTIONS_TITLE%': copy.options.pageTitle,
    '%COPY_PREVIEW_TITLE%': copy.preview.pageTitle,
  };
  return {
    name: 'passpine-copy-html',
    transformIndexHtml: {
      order: 'pre' as const,
      handler: (html: string) => Object.entries(titles).reduce((result, [token, title]) => result.replaceAll(token, title), html),
    },
  };
}
