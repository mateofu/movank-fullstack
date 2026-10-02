<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '$lib/components/Button.svelte';
  import ProductForm from '$lib/components/ProductForm.svelte';
  import Receipt from '$lib/components/Receipt.svelte';
  import * as api from '$lib/api';
  import { processCheckout } from '$lib/checkout';
  import { beginCheckout, changeQuantity, loadWorkspace, saveCheckout, updateWorkspace } from '$lib/storage';
  import { cartTotal, formatMoney } from '$lib/money';
  import { scopeOf, type Dashboard, type Identity, type Product, type Scenario, type Workspace } from '$lib/types';

  const empty = (): Workspace => ({ cartKey: null, lines: [], checkout: null, receipt: null });
  let identity = $state<Identity | null>(null);
  let workspace = $state<Workspace>(empty());
  let products = $state<Product[]>([]);
  let dashboard = $state<Dashboard | null>(null);
  let token = $state('');
  let query = $state('');
  let error = $state('');
  let loading = $state(true);
  let signingIn = $state(false);
  let busy = $state(false);
  let online = $state(true);
  let live = $state(false);
  let scenario = $state<Scenario>('APPROVED');
  let productForm = $state<ProductForm>();
  let channel: BroadcastChannel | undefined;
  let stopStream = () => {};
  let generation = 0;
  let scope = $derived(identity ? scopeOf(identity) : '');
  let filtered = $derived(products.filter((product) => `${product.name} ${product.sku}`.toLocaleLowerCase('es').includes(query.toLocaleLowerCase('es').trim())));
  let total = $derived(workspace.checkout?.sale?.total_minor ?? cartTotal(workspace.lines));
  let quantity = $derived(workspace.lines.reduce((sum, line) => sum + line.quantity, 0));
  let priceChanged = $derived(!!workspace.checkout?.sale && workspace.checkout.sale.total_minor !== workspace.checkout.confirmedTotal);

  function report(cause: unknown) {
    error = api.errorMessage(cause);
    if (cause instanceof api.ApiError && (cause.status === 401 || cause.code === 'session_changed')) {
      generation++; stopStream(); identity = null; workspace = empty(); products = []; dashboard = null; live = false; loading = false;
    }
  }

  function apply(next: Workspace, owner: string) {
    if (scope !== owner) return;
    workspace = next;
    channel?.postMessage({ type: 'workspace', scope: owner });
  }

  function updateDashboard(value: Dashboard) {
    if (!dashboard || value.date > dashboard.date || (value.date === dashboard.date && value.paid_sales >= dashboard.paid_sales)) dashboard = value;
    live = true;
  }

  async function boot(known?: Identity) {
    const current = ++generation;
    stopStream(); identity = null; workspace = empty(); products = []; dashboard = null; live = false; loading = true;
    try {
      const who = known ?? await api.me();
      const owner = scopeOf(who);
      const saved = await loadWorkspace(owner);
      if (current !== generation) return;
      identity = who; workspace = saved;
      const results = await Promise.allSettled([api.getProducts(owner), api.getDashboard(owner)]);
      if (current !== generation) return;
      if (results[0].status === 'fulfilled') products = results[0].value; else report(results[0].reason);
      if (results[1].status === 'fulfilled') updateDashboard(results[1].value); else report(results[1].reason);
      if (current !== generation) return;
      stopStream = api.watchDashboard(owner, (value) => { if (current === generation) updateDashboard(value); }, (cause) => {
        if (current !== generation) return;
        live = false;
        if (cause instanceof api.ApiError) report(cause);
      });
    } catch (cause) {
      if (current === generation && !(cause instanceof api.ApiError && cause.status === 401)) report(cause);
    } finally { if (current === generation) loading = false; }
    if (identity && workspace.checkout && !priceChanged) await resume();
  }

  async function signIn(event: SubmitEvent) {
    event.preventDefault(); signingIn = true; error = '';
    try {
      const who = await api.login(token.trim());
      token = '';
      channel?.postMessage({ type: 'session' });
      await boot(who);
    } catch (cause) { report(cause); }
    finally { signingIn = false; }
  }

  async function signOut() {
    if (busy) return;
    try {
      await api.logout(); generation++; stopStream(); identity = null; workspace = empty(); products = []; dashboard = null; error = ''; live = false;
      channel?.postMessage({ type: 'session' });
    } catch (cause) { report(cause); }
  }

  async function adjust(product: Product, delta: number) {
    const owner = scope;
    try { error = ''; apply(await changeQuantity(owner, product, delta), owner); }
    catch (cause) { report(cause); }
  }

  async function resume() {
    if (busy || !scope || !online) return;
    busy = true; error = '';
    const owner = scope;
    try { await processCheckout(owner, (next) => apply(next, owner)); }
    catch (cause) { if (scope === owner) report(cause); }
    finally { busy = false; }
  }

  async function checkout() {
    if (busy) return;
    const owner = scope;
    try { error = ''; apply(await beginCheckout(owner, scenario), owner); await resume(); }
    catch (cause) { report(cause); }
  }

  async function confirmPrice() {
    const current = workspace.checkout;
    if (!current?.sale) return;
    const owner = scope;
    try { apply(await saveCheckout(owner, current.key, { confirmedTotal: current.sale.total_minor }), owner); await resume(); }
    catch (cause) { report(cause); }
  }

  async function newSale() {
    const owner = scope;
    try { error = ''; apply(await updateWorkspace(owner, (current) => ({ ...current, receipt: null })), owner); }
    catch (cause) { report(cause); }
  }

  async function addProduct(input: { sku: string; name: string; price_minor: number }) {
    const owner = scope;
    try {
      await api.createProduct(owner, input);
      const list = await api.getProducts(owner);
      if (scope === owner) products = list;
    } catch (cause) { report(cause); throw cause; }
  }

  onMount(() => {
    online = navigator.onLine;
    channel = new BroadcastChannel('movank');
    channel.onmessage = async ({ data }) => {
      if (data?.type === 'session') { error = ''; await boot(); }
      if (data?.type === 'workspace' && data.scope === scope) {
        const owner = scope;
        try { const saved = await loadWorkspace(owner); if (scope === owner) workspace = saved; }
        catch (cause) { report(cause); }
      }
    };
    const connection = () => { online = navigator.onLine; if (online && workspace.checkout) void resume(); };
    window.addEventListener('online', connection); window.addEventListener('offline', connection);
    const timer = setInterval(() => {
      if (workspace.checkout?.payment && !priceChanged) void resume();
    }, 4000);
    void boot();
    return () => { generation++; stopStream(); channel?.close(); clearInterval(timer); window.removeEventListener('online', connection); window.removeEventListener('offline', connection); };
  });
</script>

<svelte:head><meta name="description" content="Caja de ventas Movank" /></svelte:head>

{#if loading}
  <main class="loading-screen"><img src="/favicon.svg" alt="" width="42" height="42" /><p role="status">Abriendo caja…</p></main>
{:else if !identity}
  <main class="login-page">
    <section class="login-panel">
      <div class="brand"><img src="/favicon.svg" alt="" width="36" height="36" /><span>movank</span></div>
      <h1>Entra a tu caja</h1><p class="muted">Usa el token de acceso de tu comercio.</p>
      <form onsubmit={signIn}>
        <label>Token de acceso<input type="password" bind:value={token} required autocomplete="off" placeholder="Pega tu token" /></label>
        {#if error}<p class="error-text" role="alert">{error}</p>{/if}
        <Button class="full" type="submit" disabled={signingIn}>{signingIn ? 'Entrando…' : 'Entrar'}</Button>
      </form>
      <p class="login-note">Entorno de prueba. Los pagos son simulados.</p>
    </section>
  </main>
{:else}
  <header class="topbar">
    <div class="brand"><img src="/favicon.svg" alt="" width="32" height="32" /><span>movank</span></div>
    <span class="nav-current">Caja</span>
    <div class="account"><span class="merchant-name">{identity.merchant.name}</span><button class="text-button" onclick={signOut} disabled={busy}>Cerrar sesión</button></div>
  </header>
  <main class="app-shell">
    <section class="page-heading"><div><p class="eyebrow">Punto de venta</p><h1>Nueva venta</h1></div><span class="connection" class:connected={online && live}><span class="status-dot"></span>{!online ? 'Sin conexión' : live ? 'Conectado' : 'Actualizando'}</span></section>
    {#if error}<div class="notice error" role="alert">{error}<button class="icon-button" aria-label="Cerrar aviso" onclick={() => error = ''}>×</button></div>{/if}
    {#if !online}<p class="notice" role="status">El carrito está guardado en este dispositivo. Necesitas conexión para pagar.</p>{/if}
    <section class="daily-summary" aria-label="Resumen del día">
      <div class="summary-heading"><span class="summary-mark" aria-hidden="true">↗</span><div><strong>Resumen de hoy</strong><span>Día UTC · pagos aprobados</span></div></div>
      <div class="summary-value"><span>Ventas pagadas</span><strong>{dashboard?.paid_sales ?? '—'}</strong></div>
      <div class="summary-value"><span>Total cobrado</span><strong>{dashboard ? formatMoney(dashboard.total_minor) : '—'}</strong></div>
    </section>
    <div class="workspace">
      <section class="catalog" aria-labelledby="catalog-title">
        <div class="section-heading"><div class="heading-with-count"><h2 id="catalog-title">Catálogo</h2><span class="count">{products.length}</span></div><Button variant="secondary" onclick={() => productForm?.open()} disabled={!online}>+ Nuevo producto</Button></div>
        <label class="search"><svg viewBox="0 0 24 24" width="19" height="19" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/></svg><span class="sr-only">Buscar productos</span><input bind:value={query} type="search" placeholder="Buscar por nombre o código" /></label>
        {#if !products.length}
          <div class="catalog-empty"><div class="empty-symbol" aria-hidden="true">+</div><h3>Tu catálogo está vacío</h3><p>Crea el primer producto para empezar a vender.</p><Button variant="secondary" onclick={() => productForm?.open()} disabled={!online}>Crear producto</Button></div>
        {:else if !filtered.length}
          <p class="empty-search">No hay productos que coincidan con “{query}”.</p>
        {:else}
          <div class="product-grid">
            {#each filtered as product (product.id)}
              <button class="product-card" onclick={() => adjust(product, 1)} disabled={!!workspace.checkout || busy} aria-label={`Añadir ${product.name}`}>
                <div class="product-top"><span class="product-letter" aria-hidden="true">{product.name.charAt(0).toUpperCase()}</span><span class="add-mark" aria-hidden="true">+</span></div>
                <span class="product-code">{product.sku}</span><h3>{product.name}</h3><span class="product-price">{formatMoney(product.price_minor)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>
      <aside class="cart" aria-labelledby="cart-title">
        {#if workspace.receipt}
          <Receipt sale={workspace.receipt.sale} payment={workspace.receipt.payment} onnew={newSale} />
        {:else}
          <div class="cart-heading"><h2 id="cart-title">Tu venta</h2><span class="count">{quantity}</span></div>
          {#if !workspace.lines.length}
            <div class="empty-cart"><svg viewBox="0 0 48 48" width="50" height="50" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M8 13h5l5 22h21l4-16H15M22 13V7m9 6V7"/><circle cx="21" cy="41" r="2"/><circle cx="36" cy="41" r="2"/></svg><h3>Aún no hay productos</h3><p>Elige un producto del catálogo<br />para añadirlo a la venta.</p></div>
          {:else}
            <ul class="cart-lines">
              {#each workspace.lines as line (line.product.id)}
                <li><div class="line-info"><strong>{line.product.name}</strong><span>{formatMoney(line.product.price_minor)} c/u</span></div><div class="line-bottom"><div class="quantity-control"><button aria-label={`Quitar una unidad de ${line.product.name}`} disabled={!!workspace.checkout || busy} onclick={() => adjust(line.product, -1)}>−</button><span>{line.quantity}</span><button aria-label={`Añadir una unidad de ${line.product.name}`} disabled={!!workspace.checkout || busy} onclick={() => adjust(line.product, 1)}>+</button></div><strong>{formatMoney(line.product.price_minor * line.quantity)}</strong></div></li>
              {/each}
            </ul>
          {/if}
          <div class="cart-footer">
            <div class="cart-total"><span>Total</span><strong>{formatMoney(total)}</strong></div>
            <label class="scenario-label">Pago con tarjeta · simulación<select bind:value={scenario} disabled={!!workspace.checkout || busy}><option value="APPROVED">Aprobado</option><option value="DECLINED">Rechazado</option><option value="TIMEOUT">Sin respuesta inmediata</option></select></label>
            {#if priceChanged}
              <p class="payment-note" role="status">El precio cambió. Revisa el nuevo total antes de pagar.</p><Button class="full" onclick={confirmPrice} disabled={busy || !online}>Confirmar {formatMoney(total)}</Button>
            {:else if workspace.checkout}
              <p class="payment-note" role="status">{workspace.checkout.payment ? 'El pago está por confirmar. Consultaremos la misma operación.' : 'Conservamos esta venta hasta confirmar el resultado.'}</p>
              <Button class="full" onclick={resume} disabled={busy || !online}>{busy ? 'Consultando…' : workspace.checkout.payment ? 'Consultar estado' : 'Reintentar'}</Button>
            {:else}
              <Button class="full" onclick={checkout} disabled={!workspace.lines.length || busy || !online}>{busy ? 'Procesando…' : 'Cobrar venta'}<span aria-hidden="true">→</span></Button>
            {/if}
            <p class="cart-note">{workspace.lines.length ? 'Carrito guardado en este dispositivo' : 'Los importes están en pesos colombianos'}</p>
          </div>
        {/if}
      </aside>
    </div>
  </main>
  <ProductForm bind:this={productForm} oncreate={addProduct} />
{/if}
