import { openDB, type DBSchema } from 'idb';
import { cartTotal } from './money';
import type { Checkout, Product, Scenario, Workspace } from './types';

interface Database extends DBSchema {
  workspaces: { key: string; value: Workspace };
}

const empty = (): Workspace => ({ cartKey: null, lines: [], checkout: null, receipt: null });
const database = () => openDB<Database>('movank', 1, {
  upgrade(db) { db.createObjectStore('workspaces'); }
});

export async function loadWorkspace(scope: string): Promise<Workspace> {
  const db = await database();
  try { return (await db.get('workspaces', scope)) ?? empty(); }
  finally { db.close(); }
}

export async function updateWorkspace(scope: string, change: (current: Workspace) => Workspace): Promise<Workspace> {
  const db = await database();
  const tx = db.transaction('workspaces', 'readwrite');
  try {
    const current = (await tx.store.get(scope)) ?? empty();
    const next = change(current);
    await tx.store.put(next, scope);
    await tx.done;
    return next;
  } catch (error) {
    try { tx.abort(); } catch {}
    await tx.done.catch(() => {});
    throw error;
  } finally { db.close(); }
}

export async function changeQuantity(scope: string, product: Product, delta: number): Promise<Workspace> {
  return updateWorkspace(scope, (current) => {
    if (current.checkout) throw new Error('Termina la venta en curso antes de cambiar el carrito.');
    const existing = current.lines.find((line) => line.product.id === product.id);
    const quantity = (existing?.quantity ?? 0) + delta;
    if (quantity > 10000) throw new Error('La cantidad máxima es 10000.');
    const lines = current.lines.filter((line) => line.product.id !== product.id);
    if (quantity > 0) lines.push({ product: existing?.product ?? { ...product }, quantity });
    if (lines.length > 100) throw new Error('Puedes incluir hasta 100 productos distintos.');
    cartTotal(lines);
    return { ...current, lines, cartKey: current.cartKey ?? crypto.randomUUID(), receipt: null };
  });
}

export async function beginCheckout(scope: string, scenario: Scenario): Promise<Workspace> {
  return updateWorkspace(scope, (current) => {
    if (current.checkout) return current;
    if (!current.lines.length) throw new Error('Añade un producto para continuar.');
    return {
      ...current,
      checkout: {
        key: current.cartKey ?? crypto.randomUUID(), scenario,
        items: current.lines.map((line) => ({ product_id: line.product.id, quantity: line.quantity })),
        confirmedTotal: cartTotal(current.lines), sale: null, payment: null
      }
    };
  });
}

export const saveCheckout = (scope: string, key: string, patch: Partial<Checkout>) =>
  updateWorkspace(scope, (current) => current.checkout?.key === key
    ? { ...current, checkout: { ...current.checkout, ...patch } } : current);

export const finishCheckout = (scope: string, key: string) => updateWorkspace(scope, (current) => {
  const checkout = current.checkout;
  if (checkout?.key !== key || !checkout.sale || !checkout.payment) return current;
  if (checkout.payment.status !== 'APPROVED' && checkout.payment.status !== 'DECLINED') return current;
  return { ...empty(), receipt: { sale: checkout.sale, payment: checkout.payment } };
});
