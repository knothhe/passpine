import { beforeEach, vi } from 'vitest';

// Existing interaction suites exercise Chinese UI. Language tests explicitly
// select their locale instead of depending on the test runner's host language.
beforeEach(() => {
  if (typeof navigator !== 'undefined') vi.spyOn(navigator, 'language', 'get').mockReturnValue('zh-CN');
});
