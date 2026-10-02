import { z } from 'zod';
import { catalogSchema, dashboardSchema, identitySchema, paymentSchema, productSchema, saleSchema, type Dashboard, type Product } from './types';

export class ApiError extends Error {
  constructor(public status: number, public code: string) { super(code); }
}

export async function request<T>(path: string, schema: z.ZodType<T>, scope = '', options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (scope) headers.set('X-Workspace', scope);
  if (options.body) headers.set('Content-Type', 'application/json');
  const response = await fetch(`/v1${path}`, { ...options, headers, credentials: 'same-origin', signal: options.signal ?? AbortSignal.timeout(15000) });
  const body: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const error = z.object({ error: z.string() }).safeParse(body);
    throw new ApiError(response.status, error.success ? error.data.error : 'unavailable');
  }
  const result = schema.safeParse(body);
  if (!result.success) throw new Error('La respuesta del servidor no tiene el formato esperado.');
  return result.data;
}

export const me = () => request('/me', identitySchema);
export const login = (token: string) => request('/session', identitySchema, '', { method: 'POST', body: JSON.stringify({ token }) });
export async function logout(): Promise<void> {
  const response = await fetch('/v1/session', { method: 'DELETE', credentials: 'same-origin', signal: AbortSignal.timeout(10000) });
  if (!response.ok) throw new Error('No se pudo cerrar la sesión. Inténtalo de nuevo.');
}
export const getDashboard = (scope: string) => request('/dashboard/today', dashboardSchema, scope);
export const getSale = (scope: string, id: string) => request(`/sales/${id}`, saleSchema, scope);
export const createSale = (scope: string, key: string, items: { product_id: string; quantity: number }[]) =>
  request('/sales', saleSchema, scope, { method: 'POST', headers: { 'Idempotency-Key': key }, body: JSON.stringify({ items }) });
export const paySale = (scope: string, id: string, scenario: string) =>
  request(`/sales/${id}/pay`, paymentSchema, scope, { method: 'POST', body: JSON.stringify({ method: 'CARD', scenario }) });
export const createProduct = (scope: string, input: { sku: string; name: string; price_minor: number }) =>
  request('/products', productSchema, scope, { method: 'POST', body: JSON.stringify({ ...input, currency: 'COP' }) });

export async function getProducts(scope: string): Promise<Product[]> {
  const products: Product[] = [];
  let cursor: string | null = null;
  const visited = new Set<string>();
  do {
    const page: z.infer<typeof catalogSchema> = await request(`/products?limit=100${cursor ? `&after=${cursor}` : ''}`, catalogSchema, scope);
    products.push(...page.items);
    cursor = page.next_cursor;
    if (cursor && visited.has(cursor)) throw new Error('No se pudo completar el catálogo.');
    if (cursor) visited.add(cursor);
    if (visited.size > 100) throw new Error('El catálogo supera el tamaño permitido en esta versión.');
  } while (cursor);
  return products.sort((a, b) => a.name.localeCompare(b.name, 'es'));
}

export function watchDashboard(scope: string, onValue: (value: Dashboard) => void, onError: (error: unknown) => void): () => void {
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout>;
  async function connect() {
    try {
      const response = await fetch('/v1/dashboard/stream', { headers: { 'X-Workspace': scope }, credentials: 'same-origin', signal: controller.signal });
      if (response.status === 401 || response.status === 409) throw new ApiError(response.status, response.status === 409 ? 'session_changed' : 'unauthorized');
      if (!response.ok || !response.body) throw new Error('No se pudo actualizar el resumen en vivo.');
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      try {
        while (true) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          if (buffer.length > 65536) throw new Error('Evento demasiado grande.');
          let end: number;
          while ((end = buffer.indexOf('\n\n')) !== -1) {
            const event = buffer.slice(0, end);
            buffer = buffer.slice(end + 2);
            for (const line of event.split('\n')) {
              if (line.startsWith('data: ')) onValue(dashboardSchema.parse(JSON.parse(line.slice(6))));
            }
          }
        }
      } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
    } catch (error) {
      if (!controller.signal.aborted) onError(error);
      if (error instanceof ApiError) return;
    }
    if (!controller.signal.aborted) timer = setTimeout(connect, 3000);
  }
  void connect();
  return () => { controller.abort(); clearTimeout(timer); };
}

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return 'Tu sesión terminó. Vuelve a entrar para continuar.';
    if (error.code === 'session_changed') return 'La sesión cambió en otra pestaña. Vuelve a entrar.';
    if (error.code === 'sku_conflict') return 'Ya existe un producto con ese código.';
    if (error.status === 409) return 'La operación ya existe con otros datos. No se ha iniciado otro cobro.';
    if (error.status === 400) return 'Revisa los datos antes de continuar.';
    return 'El servidor no pudo completar la operación. Puedes reintentar.';
  }
  if (error instanceof TypeError || (error instanceof DOMException && ['TimeoutError', 'AbortError'].includes(error.name))) return 'No hay respuesta del servidor. Conservamos tu venta para reintentar.';
  return error instanceof Error ? error.message : 'No se pudo completar la operación.';
}
