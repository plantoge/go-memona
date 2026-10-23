<script lang="ts">
  import type { View } from '../types';

  let { name }: { name: View | 'panel' | 'chevron' | 'logout' | 'sun' | 'moon' | 'plus' | 'copy' | 'close' | 'refresh' | 'server' | 'key' | 'edit' | 'trash' | 'zoom-in' | 'zoom-out' | 'search' | 'filter' | 'pause' | 'play' | 'fit' | 'expand' } = $props();
</script>

<svg class="icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
  {#if name === 'notes'}
    <rect x="4" y="3" width="16" height="18" rx="3" /><path d="M8 8h8M8 12h8M8 16h5" />
  {:else if name === 'projects'}
    <path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" />
  {:else if name === 'skills'}
    <path d="m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5L12 3Z" />
  {:else if name === 'keys' || name === 'server' || name === 'storage'}
    <rect x="3" y="3" width="18" height="7" rx="2" /><rect x="3" y="14" width="18" height="7" rx="2" /><path d="M7 6.5h.01M7 17.5h.01M16 6.5h2M16 17.5h2" />
  {:else if name === 'key'}
    <circle cx="8" cy="8" r="5" /><path d="m11.5 11.5 9 9M16 16l3-3M19 19l2-2" />
  {:else if name === 'graph'}
    <circle cx="5" cy="6" r="3" /><circle cx="19" cy="6" r="3" /><circle cx="12" cy="19" r="3" /><path d="M8 6h8M6.5 9l4 7m7-7-4 7" />
  {:else if name === 'zoom-in' || name === 'zoom-out'}
    <circle cx="10" cy="10" r="7" /><path d="m15 15 6 6M6 10h8" />{#if name === 'zoom-in'}<path d="M10 6v8" />{/if}
  {:else if name === 'search'}
    <circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" />
  {:else if name === 'filter'}
    <path d="M4 7h16M4 17h16" /><circle cx="9" cy="7" r="2.5" fill="var(--surface)" /><circle cx="15" cy="17" r="2.5" fill="var(--surface)" />
  {:else if name === 'pause'}
    <path d="M8 5v14M16 5v14" stroke-width="3" />
  {:else if name === 'play'}
    <path d="m8 5 11 7-11 7Z" />
  {:else if name === 'fit' || name === 'expand'}
    <path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5" />{#if name === 'fit'}<circle cx="12" cy="12" r="3" />{/if}
  {:else if name === 'activity'}
    <path d="M3 12h4l3-8 4 16 3-8h4" />
  {:else if name === 'panel'}
    <rect x="3" y="4" width="18" height="16" rx="3" /><path d="M9 4v16M5.5 8h1M5.5 12h1" />
  {:else if name === 'chevron'}
    <path d="m14 7-5 5 5 5" />
  {:else if name === 'sun'}
    <circle cx="12" cy="12" r="4" /><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5" />
  {:else if name === 'moon'}
    <path d="M20.5 14A9 9 0 0 1 10 3.5 9 9 0 1 0 20.5 14Z" />
  {:else if name === 'plus'}<path d="M12 5v14M5 12h14" />
  {:else if name === 'copy'}<rect x="8" y="8" width="12" height="13" rx="2" /><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3" />
  {:else if name === 'close'}<path d="m6 6 12 12M6 18 18 6" />
  {:else if name === 'refresh'}<path d="M20 7v5h-5M4 17v-5h5M6 6a8 8 0 0 1 13 2M5 16a8 8 0 0 0 13 2" />
  {:else if name === 'edit'}<path d="m15 4 5 5M4 20l5-1L20 8a3.5 3.5 0 0 0-5-5L4 14v6Z" />
  {:else if name === 'trash'}<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7" />
  {:else if name === 'logout'}
    <path d="M9 4H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h4M9 12h12m-4-4 4 4-4 4" />
  {/if}
</svg>
