import 'fake-indexeddb/auto';
import { describe, expect, it } from 'vitest';
import { beginCheckout, changeQuantity, finishCheckout, loadWorkspace, saveCheckout } from './storage';
import { parsePrice } from './money';
import type { Product } from './types';

const product: Product = { id: '11111111-1111-4111-8111-111111111111', sku: 'CAFE', name: 'Café', price_minor: 125050, currency: 'COP', created_at: '2026-09-30T10:00:00Z' };

describe('carrito persistente', () => {
  it('aísla usuarios y conserva cambios concurrentes', async () => {
    const a = crypto.randomUUID();
    const b = crypto.randomUUID();
    await Promise.all([changeQuantity(a, product, 1), changeQuantity(a, product, 1)]);
    expect((await loadWorkspace(a)).lines[0].quantity).toBe(2);
    expect((await loadWorkspace(b)).lines).toEqual([]);
  });

  it('dos pestañas preparan una misma operación y congelan el pedido', async () => {
    const scope = crypto.randomUUID();
    await changeQuantity(scope, product, 1);
    const [first, second] = await Promise.all([beginCheckout(scope, 'TIMEOUT'), beginCheckout(scope, 'APPROVED')]);
    expect(first.checkout?.key).toBe(second.checkout?.key);
    expect(second.checkout?.scenario).toBe('TIMEOUT');
    await expect(changeQuantity(scope, product, 1)).rejects.toThrow('Termina la venta');
    expect((await loadWorkspace(scope)).checkout).toEqual(first.checkout);
  });

  it('un resultado viejo no borra otra venta y UNKNOWN no termina el checkout', async () => {
    const scope = crypto.randomUUID();
    await changeQuantity(scope, product, 1);
    const current = await beginCheckout(scope, 'TIMEOUT');
    const key = current.checkout!.key;
    await saveCheckout(scope, 'old-key', { confirmedTotal: 999 });
    expect((await loadWorkspace(scope)).checkout?.confirmedTotal).toBe(125050);
    await finishCheckout(scope, key);
    expect((await loadWorkspace(scope)).checkout?.key).toBe(key);
  });
});

it('convierte pesos a centavos sin redondear importes inválidos', () => {
  expect(parsePrice('1250,50')).toBe(125050);
  expect(parsePrice('0.01')).toBe(1);
  expect(() => parsePrice('1.999')).toThrow();
  expect(() => parsePrice('-20')).toThrow();
  expect(() => parsePrice('0')).toThrow();
});
