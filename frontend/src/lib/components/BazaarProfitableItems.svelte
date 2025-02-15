<script>
  import { onMount } from 'svelte';

  let items = [];
  let loading = true;
  let error = null;

  // Fetch top bazaar crafts on mount
  const fetchBazaarData = async () => {
    try {
      const response = await fetch('/top_40_bazaar_crafts.json');
      if (!response.ok) throw new Error(`Bazaar Data Error: ${response.statusText}`);
      items = await response.json();
    } catch (err) {
      error = `Bazaar Error: ${err.message}`;
    } finally {
      loading = false;
    }
  };

  onMount(fetchBazaarData);

  // Utility to convert strings to Title Case
  const toTitleCase = (str) => {
    return str
      .split(' ')
      .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
      .join(' ');
  };

  // Format numbers more cleanly
  const formatNumber = (num, decimals = 1) => {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const formatted = num.toLocaleString('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0$/, '');
  };

  // Compute percent flip as (crafting_savings / crafting_cost) * 100
  const percentFlip = (item) => {
    if (!item.crafting_cost) return 0;
    return (item.crafting_savings / item.crafting_cost) * 100;
  };
</script>

<style>
  /* Global body styling */
  :global(body) {
    margin: 0;
    background-color: #0B0B16; /* Near-black with purple undertones */
    color: #e5e7eb;
    font-family: system-ui, sans-serif;
  }

  .container {
    padding: 2rem;
    max-width: 1400px;
    margin: 0 auto;
  }

  /* Grid for the card layout */
  .grid {
    display: grid;
    gap: 2rem;
    grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  }

  /* Airbnb-inspired card with space for an image */
  .card {
    background: #1a1a1a;
    border-radius: 12px;
    padding: 1.5rem;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
    transition: transform 0.2s ease, box-shadow 0.2s ease;
    text-decoration: none;
    color: inherit;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
  }

  .card:hover {
    transform: translateY(-4px);
    box-shadow: 0 8px 12px rgba(0, 0, 0, 0.4);
  }

  /* Top bar placeholder (similar to "Nearest to you" in the example) */
  .top-bar {
    margin-bottom: 1rem;
  }

  .image-wrapper {
    margin-bottom: 1rem;
  }

  .image-wrapper img {
    width: 100%;
    height: 10rem;
    object-fit: cover;
    border-radius: 8px;
  }

  .item-name {
    margin-bottom: 0.25rem;
  }

  .price {
    margin-bottom: 0.75rem;
  }

  /* Stats section (small pills) */
  .stats {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .stat-item {
    background: #12121E;
    padding: 0.3rem 0.6rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }

  .stat-item .text-xs {
    font-size: 0.75rem;
  }

  .stat-item .ml-1 {
    margin-left: 0.25rem;
  }
</style>

<div class="container">
  {#if loading}
    <div>🔄 Loading...</div>
  {:else if error}
    <div>❌ {error}</div>
  {:else}
    <div class="grid">
      {#each items as item}
        <!-- Each card links to its detailed page -->
        <a class="card" href={`/bazaaritems/${encodeURIComponent(item.item)}`}>
          <!-- Top bar (placeholder) -->
          <div class="top-bar flex justify-between items-center">
            <span class="text-sm text-gray-400">200 m Nearest to you</span>
            <button class="text-sm text-accent hover:opacity-80 transition-opacity">
              Detail
            </button>
          </div>

          <!-- Image placeholder -->
          <div class="image-wrapper">
            <img
              src="/images/placeholder.png"
              alt="Item Image"
            />
          </div>

          <!-- Item info -->
          <div class="item-info">
            <div class="item-name text-lg font-semibold text-light">
              {toTitleCase(item.item.replace(/_/g, ' '))}
            </div>
            <div class="price text-2xl font-bold text-accent">
              ◆ {formatNumber(item.profit_per_hour)}/h
            </div>
          </div>

          <!-- Stats pills -->
          <div class="stats">
            <div class="stat-item">
              <span class="text-xs text-gray-300">Craft Cost:</span>
              <span class="ml-1 text-light font-medium">
                ◆ {formatNumber(item.crafting_cost)}
              </span>
            </div>
            <div class="stat-item">
              <span class="text-xs text-gray-300">Sell Price:</span>
              <span class="ml-1 text-light font-medium">
                ◆ {formatNumber(item.sell_price)}
              </span>
            </div>
            <div class="stat-item">
              <span class="text-xs text-gray-300">Cycles/h:</span>
              <span class="ml-1 text-light font-medium">
                {formatNumber(item.cycles_per_hour)}
              </span>
            </div>
            <div class="stat-item">
              <span class="text-xs text-gray-300">Max Depth:</span>
              <span class="ml-1 text-light font-medium">
                {item.longest_step_count}
              </span>
            </div>
            <div class="stat-item">
              <span class="text-xs text-gray-300">Savings:</span>
              <span class="ml-1 text-light font-medium">
                ▲ {formatNumber(item.crafting_savings)}
              </span>
            </div>
            <div class="stat-item">
              <span class="text-xs text-gray-300">% Flip:</span>
              <span class="ml-1 text-light font-medium">
                {formatNumber(percentFlip(item))}%
              </span>
            </div>
          </div>
        </a>
      {/each}
    </div>
  {/if}
</div>
