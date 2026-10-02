import { z } from 'zod';

const money = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER);
export const productSchema = z.object({
  id: z.uuid(), sku: z.string(), name: z.string(),
  price_minor: money.positive(), currency: z.literal('COP'), created_at: z.string()
});
export const identitySchema = z.object({
  user_id: z.uuid(), merchant: z.object({ id: z.uuid(), name: z.string() })
});
export const scenarioSchema = z.enum(['APPROVED', 'DECLINED', 'TIMEOUT']);
export const paymentSchema = z.object({
  id: z.uuid(), sale_id: z.uuid(), method: z.literal('CARD'), scenario: scenarioSchema,
  status: z.enum(['PENDING', 'UNKNOWN', 'APPROVED', 'DECLINED']), amount_minor: money,
  checks: z.number().int().nonnegative(), last_error: z.string().nullable(),
  created_at: z.string(), updated_at: z.string(), completed_at: z.string().nullable()
});
export const saleSchema = z.object({
  id: z.uuid(), total_minor: money, currency: z.literal('COP'), created_at: z.string(),
  items: z.array(z.object({
    product_id: z.uuid(), quantity: z.number().int().positive(), product_name: z.string(),
    unit_price_minor: money, subtotal_minor: money
  })),
  payment: paymentSchema.nullable().optional()
});
export const dashboardSchema = z.object({
  date: z.string(), currency: z.literal('COP'), paid_sales: money, total_minor: money
});
export const catalogSchema = z.object({ items: z.array(productSchema), next_cursor: z.uuid().nullable() });

export type Product = z.infer<typeof productSchema>;
export type Identity = z.infer<typeof identitySchema>;
export type Payment = z.infer<typeof paymentSchema>;
export type Sale = z.infer<typeof saleSchema>;
export type Dashboard = z.infer<typeof dashboardSchema>;
export type Scenario = z.infer<typeof scenarioSchema>;
export type CartLine = { product: Product; quantity: number };
export type Checkout = {
  key: string;
  items: { product_id: string; quantity: number }[];
  scenario: Scenario;
  confirmedTotal: number;
  sale: Sale | null;
  payment: Payment | null;
};
export type Workspace = {
  cartKey: string | null;
  lines: CartLine[];
  checkout: Checkout | null;
  receipt: { sale: Sale; payment: Payment } | null;
};

export const scopeOf = (identity: Identity) => `${identity.merchant.id}:${identity.user_id}`;
