import type { CartLine } from './types';

export function formatMoney(minor: number): string {
  return new Intl.NumberFormat('es-CO', {
    style: 'currency', currency: 'COP', minimumFractionDigits: 0, maximumFractionDigits: 2
  }).format(minor / 100);
}

export function parsePrice(value: string): number {
  if (!/^\d{1,11}([.,]\d{1,2})?$/.test(value.trim())) throw new Error('Escribe un precio válido, por ejemplo 12500 o 12500,50.');
  const [whole, fraction = ''] = value.trim().replace(',', '.').split('.');
  const result = Number(whole) * 100 + Number(fraction.padEnd(2, '0'));
  if (result < 1 || result > 1_000_000_000_000) throw new Error('El precio está fuera del rango permitido.');
  return result;
}

export function cartTotal(lines: CartLine[]): number {
  const result = lines.reduce((sum, line) => sum + line.product.price_minor * line.quantity, 0);
  if (!Number.isSafeInteger(result) || result > 1_000_000_000_000) throw new Error('El total supera el límite de la venta.');
  return result;
}
