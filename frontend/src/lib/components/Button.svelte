<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';
  type Props = HTMLButtonAttributes & { variant?: 'primary' | 'secondary' | 'danger' };
  let { children, variant = 'primary', type = 'button', class: extra = '', ...rest }: Props = $props();
</script>

<button {type} class={`button ${variant} ${extra}`} {...rest}>
  {#if children}{@render children()}{/if}
</button>
