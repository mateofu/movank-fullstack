import { createSale, getSale, paySale } from './api';
import { finishCheckout, loadWorkspace, saveCheckout } from './storage';
import type { Workspace } from './types';

export async function processCheckout(scope: string, onChange: (workspace: Workspace) => void): Promise<void> {
  let workspace = await loadWorkspace(scope);
  let checkout = workspace.checkout;
  if (!checkout) return;
  if (!checkout.sale) {
    const sale = await createSale(scope, checkout.key, checkout.items);
    workspace = await saveCheckout(scope, checkout.key, { sale });
    onChange(workspace);
    checkout = workspace.checkout;
  }
  if (!checkout?.sale || checkout.sale.total_minor !== checkout.confirmedTotal) return;
  const payment = checkout.payment
    ? (await getSale(scope, checkout.sale.id)).payment
    : await paySale(scope, checkout.sale.id, checkout.scenario);
  if (!payment) throw new Error('No se pudo confirmar el estado del pago.');
  workspace = await saveCheckout(scope, checkout.key, { payment });
  onChange(workspace);
  if (payment.status === 'APPROVED' || payment.status === 'DECLINED') onChange(await finishCheckout(scope, checkout.key));
}
