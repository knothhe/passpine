import type { Copy } from '../constants/copy';
import { systemCopy } from './i18n';

export class PassPineError extends Error {
  constructor(public code: string, public detail?: string) { super(messageFor(code, detail, systemCopy())); }
}
function messageFor(code: string, detail: string | undefined, copy: Copy): string {
  const messages: Readonly<Record<string, string>> = copy.errors;
  return (Object.hasOwn(messages, code) ? messages[code] : undefined) ?? detail ?? copy.operationFailed;
}
export function errorMessage(error: unknown, copy: Copy = systemCopy()): string {
  if (error instanceof PassPineError) return messageFor(error.code, error.detail, copy);
  return error instanceof Error ? error.message : copy.operationFailed;
}
