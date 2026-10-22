declare const __PASSPINE_BUILD_CHANNEL__: 'dev' | 'release';

// WXT replaces this constant in extension bundles. Node-side catalog consumers
// (HTML title generation) use the same build environment; direct source runs
// and previews remain development builds.
export const BUILD_CHANNEL = typeof __PASSPINE_BUILD_CHANNEL__ !== 'undefined'
  ? __PASSPINE_BUILD_CHANNEL__
  : typeof process !== 'undefined' && process.env.PASSPINE_BUILD_CHANNEL === 'release' ? 'release' : 'dev';
