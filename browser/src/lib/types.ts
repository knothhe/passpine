export const HOST = 'com.knothhe.passpine';
export const PROTOCOL = 1;

export type NativeAction = 'configure' | 'status' | 'list' | 'fetch' | 'save' | 'delete' | 'lock' | 'unlock' | 'generate' | 'copy';
export interface NativeRequest {
  protocol: 1;
  requestId?: string;
  action: NativeAction;
  vaultId?: string;
  entry?: string;
  contents?: string;
  mode?: 'create' | 'edit';
  destination?: string;
  revision?: string;
  includeEntries?: boolean;
  interactive?: boolean;
  purpose?: 'view' | 'fill' | 'copyPassword' | 'copyUsername' | 'copyOTP';
  field?: 'password' | 'username' | 'otp';
  length?: number;
  lowercase?: boolean;
  uppercase?: boolean;
  numbers?: boolean;
  symbols?: boolean;
  excludeAmbiguous?: boolean;
}
export interface NativeResponse<T = unknown> { protocol: number; requestId?: string; status: 'ok' | 'error'; data?: T; code?: string; message?: string }
export interface EntryMetadata { path: string; name: string; folder: string }
export interface VaultState { configured: boolean; vaultId: string; vaultName: string; locked: boolean; entries: EntryMetadata[]; entriesVersion?: number; capabilities: string[] }
export type SessionStatus = Pick<VaultState, 'configured' | 'vaultId' | 'vaultName' | 'locked' | 'entriesVersion'>;
export interface TabContext { id: number; url: string; hostname: string; secure: boolean }
export interface Entry { path: string; title: string; folder: string; matched: boolean }
export interface Snapshot extends VaultState { tab?: TabContext; matches: string[] }
export interface EntryDocument { contents: string; revision: string; otp?: string; otpSeconds?: number; session?: SessionStatus }
export interface Credentials { password: string; username: string; urls: string[]; otp?: string; notes: string; fields: Array<{ key: string; value: string }> }
export interface GeneratorOptions { length: number; lowercase: boolean; uppercase: boolean; numbers: boolean; symbols: boolean; excludeAmbiguous: boolean }
export type UIRequest =
  | { action: 'snapshot' }
  | { action: 'status' }
  | { action: 'lock' }
  | { action: 'unlock' }
  | { action: 'fetch'; vaultId: string; entry: string; interactive?: boolean }
  | { action: 'save'; vaultId: string; entry: string; contents: string; mode: 'create' | 'edit'; destination?: string; revision?: string }
  | { action: 'delete'; vaultId: string; entry: string; revision: string }
  | { action: 'fill'; vaultId: string; entry: string; tab: TabContext }
  | { action: 'associate'; vaultId: string; entry: string; tab: TabContext }
  | { action: 'generate'; options: GeneratorOptions }
  | { action: 'copy'; vaultId: string; entry: string; field: 'password' | 'username' | 'otp' };

export interface Client {
  request<T>(request: UIRequest): Promise<T>;
  openOptions(): Promise<void>;
  copy(text: string): Promise<void>;
  readonly demo: boolean;
}
