export class APIError extends Error {
  status: number;
  stale: boolean;
  constructor(message: string, status = 0, stale = false) {
    super(message); this.status = status; this.stale = stale;
  }
}

// Session generation invalidates every in-flight response on logout/session expiry.
// Tokens live only in memory, never localStorage or the frontend bundle.
export function createAPI(onExpired: () => void) {
  let csrf = '';
  let generation = 0;
  return {
    setSession(token: string) { generation++; csrf = token; },
    reset() { generation++; csrf = ''; },
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
      const current = generation;
      const headers: Record<string, string> = { Accept: 'application/json' };
      if (body !== undefined) headers['Content-Type'] = 'application/json';
      if (method !== 'GET' && csrf) headers['X-CSRF-Token'] = csrf;
      let response: Response;
      try {
        response = await fetch(path, { method, headers, credentials: 'same-origin', cache: 'no-store', body: body === undefined ? undefined : JSON.stringify(body) });
      } catch {
        if (current !== generation) throw new APIError('Respons lama diabaikan.', 0, true);
        throw new APIError('Tidak dapat terhubung ke server. Periksa koneksi lalu coba lagi.');
      }
      let data: unknown;
      try { data = await response.json(); } catch { data = null; }
      if (current !== generation) throw new APIError('Respons lama diabaikan.', 0, true);
      if (!response.ok) {
        if (response.status === 401 && csrf) onExpired();
        const message = data && typeof data === 'object' && 'error' in data && typeof data.error === 'string' ? data.error : `Permintaan gagal (HTTP ${response.status}).`;
        throw new APIError(message, response.status);
      }
      return data as T;
    },
  };
}
export function array<T>(data: T[] | null): T[] {
  if (data === null) return [];
  if (!Array.isArray(data)) throw new Error('Format respons server tidak sesuai. Coba muat ulang data.');
  return data;
}
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Terjadi kesalahan. Silakan coba lagi.';
}
export function isStale(error: unknown): boolean { return error instanceof APIError && error.stale; }
