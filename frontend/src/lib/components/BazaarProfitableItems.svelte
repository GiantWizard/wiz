<script>
  import { onMount } from 'svelte';
  
  let items = [];
  let loading = true;
  let error = null;

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
  .container {
    padding: 1rem;
    max-width: 1400px;
    margin: 0 auto;
  }
  .grid {
    display: grid;
    gap: 1rem;
    grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  }
  /* Use <a> for clickable cards to ensure proper semantics */
  .card {
    display: block;
    text-decoration: none;
    background: white;
    border-radius: 8px;
    padding: 1rem;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    border-left: 3px solid #10B981;
    font-size: 0.9em;
    color: inherit;
  }
  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }
  .item-name {
    font-weight: 600;
    color: #1F2937;
    font-size: 1.1em;
  }
  .profit {
    color: #059669;
    font-weight: 700;
    font-size: 1em;
    white-space: nowrap;
  }
  .stats-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }
  .stat {
    display: flex;
    justify-content: space-between;
    padding: 0.3rem 0;
    border-bottom: 1px solid #F3F4F6;
  }
  .stat-label {
    color: #6B7280;
  }
  .stat-value {
    color: #1F2937;
    font-weight: 500;
  }
  .percent-flip {
    background: #f0f9ff;
    color: #1e40af;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-weight: bold;
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
        <!-- Use an anchor (<a>) for navigation -->
        <a class="card" href={`/bazaaritems/${encodeURIComponent(item.item)}`}>
          <div class="header">
            <div class="item-name">{item.item.replace(/_/g, ' ')}</div>
            <div class="profit">⏣ {formatNumber(item.profit_per_hour)}/h</div>
          </div>
          <div class="stats-grid">
            <div class="stat">
              <span class="stat-label">Craft Cost</span>
              <span class="stat-value">⏣ {formatNumber(item.crafting_cost)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Sell Price</span>
              <span class="stat-value">⏣ {formatNumber(item.sell_price)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Cycles</span>
              <span class="stat-value">{formatNumber(item.cycles_per_hour)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Max Depth</span>
              <span class="stat-value">{item.longest_step_count}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Savings</span>
              <span class="stat-value">▲ {formatNumber(item.crafting_savings)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">% Flip</span>
              <span class="stat-value percent-flip">{formatNumber(percentFlip(item))}%</span>
            </div>
          </div>
        </a>
      {/each}
    </div>
  {/if}
</div>
