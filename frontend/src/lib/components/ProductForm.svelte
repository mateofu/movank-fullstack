<script lang="ts">
  import Button from './Button.svelte';
  import { parsePrice } from '../money';
  import { errorMessage } from '../api';

  let { oncreate }: { oncreate: (input: { name: string; sku: string; price_minor: number }) => Promise<void> } = $props();
  let dialog: HTMLDialogElement;
  let name = $state('');
  let sku = $state('');
  let price = $state('');
  let error = $state('');
  let saving = $state(false);

  export function open() { error = ''; dialog.showModal(); }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    saving = true;
    error = '';
    try {
      await oncreate({ name: name.trim(), sku: sku.trim(), price_minor: parsePrice(price) });
      name = ''; sku = ''; price = '';
      dialog.close();
    } catch (cause) { error = errorMessage(cause); }
    finally { saving = false; }
  }
</script>

<dialog bind:this={dialog} aria-labelledby="product-title">
  <form onsubmit={submit} class="product-form">
    <div class="section-heading"><h2 id="product-title">Nuevo producto</h2><button type="button" class="icon-button" aria-label="Cerrar formulario" onclick={() => dialog.close()}>×</button></div>
    <label>Nombre<input bind:value={name} required maxlength="120" placeholder="Café americano" autocomplete="off" /></label>
    <label>Código<input bind:value={sku} required maxlength="64" pattern={'[A-Za-z0-9][A-Za-z0-9._\\-]{0,63}'} placeholder="CAFE-01" autocomplete="off" /></label>
    <label>Precio en pesos<input bind:value={price} required inputmode="decimal" placeholder="5000" autocomplete="off" /></label>
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}
    <div class="form-actions"><Button variant="secondary" onclick={() => dialog.close()}>Cancelar</Button><Button type="submit" disabled={saving}>{saving ? 'Guardando…' : 'Guardar producto'}</Button></div>
  </form>
</dialog>
