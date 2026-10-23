import { PassPineError } from './errors';
import { unwrap } from './protocol';
import type { NativeRequest } from './types';

export interface NativePort {
  postMessage(message: unknown): void;
  disconnect(): void;
  onMessage: { addListener(callback: (message: any) => void): void };
  onDisconnect: { addListener(callback: () => void): void };
}
type Pending = { resolve(value: unknown): void; reject(error: unknown): void; timer: ReturnType<typeof setTimeout> };

// One native process per background lifetime. Reconnect only for a new explicit request;
// an interrupted mutation is rejected and never replayed.
export class NativeConnection {
  private closed = false;
  private port?: NativePort;
  private pending = new Map<string, Pending>();
  private configuring?: Promise<unknown>;
  private interactions = new Set<Promise<unknown>>();
  constructor(private connect: () => NativePort, private timeoutMs = 125_000) {}

  request<T>(request: NativeRequest): Promise<T> {
    if (this.closed) return Promise.reject(new PassPineError('native_connection_failed'));
    if (request.action === 'configure' && this.configuring) return this.configuring as Promise<T>;
    const operation = this.sendWhenReady<T>(request);
    if (request.interactive && ['fetch', 'copy', 'unlock'].includes(request.action)) {
      this.interactions.add(operation);
      const clear = () => { this.interactions.delete(operation); };
      operation.then(clear, clear);
    }
    if (request.action === 'configure') {
      this.configuring = operation;
      const clear = () => { if (this.configuring === operation) this.configuring = undefined; };
      operation.then(clear, clear);
    }
    return operation;
  }
  disconnect(): void {
    this.closed = true;
    const port = this.port;
    if (port) this.failPort(port);
    this.configuring = undefined;
    this.interactions.clear();
    try { port?.disconnect(); } catch {}
  }
  private async sendWhenReady<T>(request: NativeRequest): Promise<T> {
    if (this.closed) throw new PassPineError('native_connection_failed');
    if (request.action === 'status' || request.action === 'configure') {
      // The native host processes requests in order. Start short metadata
      // deadlines only after any system authentication dialog has finished.
      while (this.interactions.size) await Promise.allSettled([...this.interactions]);
    }
    if (this.closed) throw new PassPineError('native_connection_failed');
    return this.send<T>(request);
  }
  private ensurePort(): NativePort {
    if (this.port) return this.port;
    const port = this.connect();
    this.port = port;
    port.onMessage.addListener(message => {
      if (port !== this.port || typeof message?.requestId !== 'string') return;
      const pending = this.pending.get(message.requestId);
      if (!pending) return;
      this.pending.delete(message.requestId); clearTimeout(pending.timer);
      try { pending.resolve(unwrap(message)); } catch (error) { pending.reject(error); }
    });
    port.onDisconnect.addListener(() => {
      this.failPort(port);
    });
    return port;
  }
  private send<T>(request: NativeRequest): Promise<T> {
    return new Promise((resolve, reject) => {
      const requestId = crypto.randomUUID();
      let port: NativePort;
      try { port = this.ensurePort(); } catch { reject(new PassPineError('native_connection_failed')); return; }
      const deadline = Math.min(this.timeoutMs, request.action === 'status' ? 4000 : request.action === 'configure' ? 10000 : this.timeoutMs);
      const timer = setTimeout(() => { this.pending.delete(requestId); reject(new PassPineError('transport_error')); }, deadline);
      this.pending.set(requestId, { resolve: value => resolve(value as T), reject, timer });
      try { port.postMessage({ ...request, requestId }); }
      catch { this.failPort(port); port.disconnect(); }
    });
  }
  private failPort(port: NativePort) {
    if (this.port !== port) return;
    this.port = undefined;
    for (const request of this.pending.values()) { clearTimeout(request.timer); request.reject(new PassPineError('native_connection_failed')); }
    this.pending.clear();
  }
}
