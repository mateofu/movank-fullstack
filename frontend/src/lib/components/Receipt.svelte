<script lang="ts">
  import type { Payment, Sale } from '../types';
  import { formatMoney } from '../money';
  import Button from './Button.svelte';

  let { sale, payment, onnew }: { sale: Sale; payment: Payment; onnew: () => void } = $props();
  let approved = $derived(payment.status === 'APPROVED');
</script>

<section class="receipt" aria-label="Resultado del pago">
  <div class:approved class:declined={!approved} class="result-icon" aria-hidden="true">{approved ? '✓' : '×'}</div>
  <p class="eyebrow">Resultado</p>
  <h2>{approved ? 'Pago aprobado' : 'Pago rechazado'}</h2>
  <p class="muted">{approved ? 'La venta quedó registrada.' : 'El proveedor no aprobó el pago. No se cobró esta venta.'}</p>
  <div class="receipt-lines">
    {#each sale.items as item (item.product_id)}<div><span>{item.quantity} × {item.product_name}</span><span>{formatMoney(item.subtotal_minor)}</span></div>{/each}
    <div class="receipt-total"><strong>Total</strong><strong>{formatMoney(sale.total_minor)}</strong></div>
  </div>
  <dl class="receipt-meta"><div><dt>Venta</dt><dd>{sale.id.slice(0, 8).toUpperCase()}</dd></div><div><dt>Método</dt><dd>Tarjeta</dd></div><div><dt>Fecha</dt><dd>{new Date(sale.created_at).toLocaleString('es-CO', { dateStyle: 'short', timeStyle: 'short' })}</dd></div></dl>
  <Button class="full" onclick={onnew}>Nueva venta</Button>
</section>
