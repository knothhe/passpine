import { describe, expect, it } from 'vitest';
import { copyFor, resolveLocale } from './i18n';
import { errorMessage, PassPineError } from './errors';

describe('client-compatible locales', () => {
  it.each([
    ['system', 'zh_CN.UTF-8', 'zh-CN'], ['system', 'zh-Hans', 'zh-CN'], ['system', 'zh-SG', 'zh-CN'],
    ['system', 'zh-Hans-TW', 'zh-CN'], ['system', 'zh-Latn', 'en'], ['system', 'zh-Hant-CN', 'en'],
    ['system', 'zh-TW', 'en'], ['system', 'zh-HK', 'en'], ['system', 'zh-MO', 'en'],
    ['system', 'en-GB', 'en'], ['system', 'fr-FR', 'en'], ['', '', 'en'], ['en', 'zh-CN', 'en'], ['zh-CN', 'en-US', 'zh-CN'],
  ])('resolves %s with system %s to %s', (preference, system, expected) => {
    expect(resolveLocale(preference, system)).toBe(expected);
  });

  it('formats complete English messages and preserves Chinese names, domains and notes in arguments', () => {
    const en = copyFor('en'), value = '中文账号 / Password <保留原文>';
    expect(en.accounts.count(1)).toBe('1 account');
    expect(en.accounts.count(2)).toBe('2 accounts');
    expect(en.accounts.copyPassword(value)).toBe(`Copy password for ${value}`);
    expect(en.vault.trashDescription(value)).toContain(value);
    expect(en.vault.fillResult(1, '例子.test', false)).toBe('Filled 1 login field, but could not link to 例子.test. Try again.');
    expect(copyFor('zh-CN').accounts.count(2)).toBe('2 个账号');
  });

  it('translates protocol errors at display time regardless of the native message language', () => {
    const error = new PassPineError('locked', '密码库已锁定，请在客户端解锁');
    expect(errorMessage(error, copyFor('en'))).toBe('The vault is locked. Unlock it in the client and try again.');
    expect(errorMessage(error, copyFor('zh-CN'))).toBe('密码库已锁定。请在客户端解锁后重试。');
    expect(errorMessage(new PassPineError('operation_failed', '原始中文错误'), copyFor('en'))).not.toContain('中文');
    expect(errorMessage(new PassPineError('future_error', 'Custom diagnostic'), copyFor('en'))).toBe('Custom diagnostic');
    expect(errorMessage(null, copyFor('en'))).toBe('The operation did not complete. Try again.');
  });

  it.each(['constructor', 'toString', '__proto__'])('treats inherited property %s as an unknown error code', code => {
    expect(errorMessage(new PassPineError(code, 'Custom diagnostic'), copyFor('en'))).toBe('Custom diagnostic');
    expect(errorMessage(new PassPineError(code), copyFor('zh-CN'))).toBe('操作未完成，请重试。');
  });
});
