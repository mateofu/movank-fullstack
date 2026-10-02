import { test as base, expect, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve('..');
const docker = (...args: string[]) => execFileSync('docker', ['compose', ...args], { cwd: root, encoding: 'utf8' }).trim();
const sql = (query: string) => docker('exec', '-T', 'postgres', 'psql', '-X', '-qAt', '-U', 'movank', '-d', 'movank', '-v', 'ON_ERROR_STOP=1', '-c', query);
type Shop = { id: string; token: string; other: string; otherToken: string; productId: string };

const test = base.extend<{ shop: Shop }>({
  shop: async ({ request }, use) => {
    const id = randomUUID();
    const other = randomUUID();
    sql(`INSERT INTO public.merchants(id,name) VALUES('${id}','Tienda Santa Clara'),('${other}','Otro comercio');`);
    try {
      const token = docker('exec', '-T', 'backend', '/api', 'token', id, randomUUID());
      const otherToken = docker('exec', '-T', 'backend', '/api', 'token', other, randomUUID());
      let productId = '';
      for (const [sku, name, price] of [
        ['CAFE-01', 'Café americano', 500000], ['CAFE-02', 'Cappuccino', 750000],
        ['PAN-01', 'Croissant', 650000], ['COM-01', 'Sándwich de jamón', 1400000],
        ['BEB-01', 'Jugo de naranja', 800000], ['BEB-02', 'Agua mineral', 350000]
      ] as const) {
        const response = await request.post('/v1/products', { headers: { Authorization: `Bearer ${token}` }, data: { sku, name, price_minor: price, currency: 'COP' } });
        expect(response.status()).toBe(201);
        const product = await response.json();
        if (!productId) productId = product.id;
      }
      await use({ id, token, other, otherToken, productId });
    } finally {
      sql(`DELETE FROM public.outbox WHERE merchant_id IN ('${id}','${other}'); DELETE FROM public.payment_checks WHERE merchant_id IN ('${id}','${other}'); DELETE FROM public.provider_operations WHERE merchant_id IN ('${id}','${other}'); DELETE FROM public.payments WHERE merchant_id IN ('${id}','${other}'); DELETE FROM public.sale_items WHERE merchant_id IN ('${id}','${other}'); DELETE FROM public.sales WHERE merchant_id IN ('${id}','${other}'); DELETE FROM public.products WHERE merchant_id IN ('${id}','${other}'); DELETE FROM public.merchants WHERE id IN ('${id}','${other}');`);
    }
  }
});

async function signIn(page: Page, token: string) {
  await page.goto('/');
  await page.getByLabel('Token de acceso').fill(token);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Nueva venta' })).toBeVisible();
}

test('carrito persistente, sesión segura y aislamiento al cambiar de comercio', async ({ page, context, shop }) => {
  await signIn(page, shop.token);
  await page.getByRole('button', { name: 'Añadir Café americano', exact: true }).click();
  await page.getByRole('button', { name: 'Añadir una unidad de Café americano', exact: true }).click();
  await page.reload();
  await expect(page.locator('.quantity-control span')).toHaveText('2');
  const cookie = (await context.cookies()).find((item) => item.name === 'movank_session');
  expect(cookie?.httpOnly).toBe(true);
  expect(cookie?.sameSite).toBe('Strict');
  expect(await page.evaluate(() => document.cookie)).not.toContain('movank_session');
  expect(await page.evaluate(() => localStorage.length)).toBe(0);
  mkdirSync(resolve(root, '.scratch'), { recursive: true });
  await page.screenshot({ path: resolve(root, '.scratch/frontend-desktop.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: resolve(root, '.scratch/frontend-mobile.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('button', { name: 'Cerrar sesión' }).click();
  await expect(page.getByLabel('Token de acceso')).toBeVisible();
  await signIn(page, shop.otherToken);
  await expect(page.getByText('Tu catálogo está vacío')).toBeVisible();
  await expect(page.getByText('Aún no hay productos')).toBeVisible();
  await page.getByRole('button', { name: 'Cerrar sesión' }).click();
  await expect(page.getByLabel('Token de acceso')).toBeVisible();
  await signIn(page, shop.token);
  await expect(page.locator('.quantity-control span')).toHaveText('2');
});

test('dos pestañas cobran una sola venta y reciben el dashboard', async ({ page, context, shop }) => {
  await signIn(page, shop.token);
  await page.getByRole('button', { name: 'Añadir Café americano', exact: true }).click();
  const other = await context.newPage();
  await other.goto('/');
  await expect(other.locator('.quantity-control span')).toHaveText('1');
  const firstButton = await page.getByRole('button', { name: 'Cobrar venta' }).elementHandle();
  const secondButton = await other.getByRole('button', { name: 'Cobrar venta' }).elementHandle();
  await Promise.all([firstButton!.evaluate((button: HTMLButtonElement) => button.click()), secondButton!.evaluate((button: HTMLButtonElement) => button.click())]);
  await expect(page.getByRole('heading', { name: 'Pago aprobado' })).toBeVisible();
  await expect(other.getByRole('heading', { name: 'Pago aprobado' })).toBeVisible();
  await expect(page.locator('.summary-value').first().locator('strong')).toHaveText('1');
  expect(sql(`SELECT count(*) FROM public.sales WHERE merchant_id='${shop.id}'`)).toBe('1');
  expect(sql(`SELECT count(*) FROM public.provider_operations WHERE merchant_id='${shop.id}'`)).toBe('1');
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Pago aprobado' })).toBeVisible();
  await other.close();
});

test('una respuesta perdida se recupera tras recargar sin duplicar la venta', async ({ page, shop }) => {
  await signIn(page, shop.token);
  await page.getByRole('button', { name: 'Añadir Café americano', exact: true }).click();
  let lost = false;
  await page.route('**/v1/sales', async (route) => {
    if (lost) return route.continue();
    lost = true;
    await route.fetch();
    await route.abort('failed');
  });
  await page.getByRole('button', { name: 'Cobrar venta' }).click();
  await expect(page.getByRole('alert')).toContainText('Conservamos');
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Pago aprobado' })).toBeVisible();
  expect(sql(`SELECT count(*) FROM public.sales WHERE merchant_id='${shop.id}'`)).toBe('1');
  expect(sql(`SELECT count(*) FROM public.provider_operations WHERE merchant_id='${shop.id}'`)).toBe('1');
});

test('un timeout queda por confirmar y se resuelve sin otro cobro', async ({ page, shop }) => {
  await signIn(page, shop.token);
  await page.getByRole('button', { name: 'Añadir Café americano', exact: true }).click();
  await page.getByLabel('Pago con tarjeta').selectOption('TIMEOUT');
  await page.getByRole('button', { name: 'Cobrar venta' }).click();
  await expect(page.getByText('El pago está por confirmar.', { exact: false })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Pago rechazado' })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Pago aprobado' })).toBeVisible({ timeout: 25000 });
  expect(sql(`SELECT count(*) FROM public.provider_operations WHERE merchant_id='${shop.id}'`)).toBe('1');
});

test('rechazo y confirmación de un precio cambiado', async ({ page, shop }) => {
  await signIn(page, shop.token);
  await page.getByRole('button', { name: 'Añadir Café americano', exact: true }).click();
  sql(`UPDATE public.products SET price_minor=600000 WHERE merchant_id='${shop.id}' AND id='${shop.productId}'`);
  await page.getByLabel('Pago con tarjeta').selectOption('DECLINED');
  await page.getByRole('button', { name: 'Cobrar venta' }).click();
  await expect(page.getByText('El precio cambió.', { exact: false })).toBeVisible();
  expect(sql(`SELECT count(*) FROM public.provider_operations WHERE merchant_id='${shop.id}'`)).toBe('0');
  await page.getByRole('button', { name: /Confirmar/ }).click();
  await expect(page.getByRole('heading', { name: 'Pago rechazado' })).toBeVisible();
  await expect(page.locator('.summary-value').first().locator('strong')).toHaveText('0');
});

test('creación de productos y texto escapado', async ({ page, shop }) => {
  await signIn(page, shop.token);
  await page.getByRole('button', { name: 'Nuevo producto', exact: false }).click();
  await page.getByLabel('Nombre', { exact: true }).fill('<img src=x onerror=alert(1)>');
  await page.getByLabel('Código', { exact: true }).fill('NEW-01');
  await page.getByLabel('Precio en pesos').fill('1250,50');
  await page.getByRole('button', { name: 'Guardar producto' }).click();
  await expect(page.getByRole('button', { name: 'Añadir <img src=x onerror=alert(1)>', exact: true })).toBeVisible();
  await expect(page.locator('img[src="x"]')).toHaveCount(0);
});
