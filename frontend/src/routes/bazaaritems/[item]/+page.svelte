<script context="module">
  /** @type {import('./$types').PageLoad} */
  export async function load({ params, fetch }) {
    console.log("Load function START. Received params:", params);

    // Decode the parameter (which is the item name)
    const decodedItem = decodeURIComponent(params.item);
    console.log("Decoded item name:", decodedItem);

    // Fetch the JSON file from the static folder
    try {
      const res = await fetch('/top_40_bazaar_crafts.json');
      console.log("Fetch response status:", res.status);

      if (!res.ok) {
        console.error("Error fetching JSON. Status:", res.status);
        throw new Error('Could not fetch profitable items data');
      }

      const items = await res.json();
      console.log("Items loaded from JSON:", items);

      // Debug: print the top craft from the list
      if (Array.isArray(items) && items.length > 0) {
        console.log("Top craft:", items[0]);
      } else {
        console.warn("The fetched JSON array is empty or invalid.");
      }

      if (!Array.isArray(items)) {
        console.error("Fetched JSON is not an array:", items);
        throw new Error('Fetched JSON is not an array');
      }

      // Search for the item using the decoded name
      const foundItem = items.find(i => i.item === decodedItem);
      console.log("Search result for item", decodedItem, ":", foundItem);

      if (!foundItem) {
        console.error("Item not found in JSON:", decodedItem);
        throw new Error(`Item ${decodedItem} not found`);
      }

      console.log("Load function returning data:", { props: { item: foundItem } });
      return {
        props: {
          item: foundItem
        }
      };
    } catch (error) {
      console.error("Load function caught an error:", error);
      throw error;
    }
  }
</script>

<script>
  import RecipeTree from '$lib/components/RecipeTree.svelte';

  /** @type {import('./$types').PageData} */
  export let data;
  $: item = data.item;
  
  console.log("Instance script received data:", data);
  console.log("Item in instance script:", item);

  const formatNumber = (num, decimals = 1) => {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const formatted = num.toLocaleString('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0$/, '');
  };

  const percentFlip = (item) => {
    if (!item.crafting_cost) return 0;
    return (item.crafting_savings / item.crafting_cost) * 100;
  };
</script>

<style>
  .container {
    padding: 1rem;
    max-width: 800px;
    margin: 0 auto;
  }
  .header {
    display: flex;
    flex-direction: column;
    margin-bottom: 1rem;
  }
  .item-name {
    font-weight: 600;
    font-size: 1.5em;
    color: #1F2937;
    margin-bottom: 0.5rem;
  }
  .profit {
    color: #059669;
    font-weight: 700;
    font-size: 1.2em;
  }
  .stats-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.5rem;
    margin-bottom: 1rem;
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

{#if item}
  <div class="container">
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
      <div class="stat">
        <span class="stat-label">Buy Fill Time</span>
        <span class="stat-value">{formatNumber(item.buy_fill_time)}s</span>
      </div>
      <div class="stat">
        <span class="stat-label">Sell Fill Time</span>
        <span class="stat-value">{formatNumber(item.sell_fill_time)}s</span>
      </div>
      <div class="stat">
        <span class="stat-label">Effective Cycle Time</span>
        <span class="stat-value">{formatNumber(item.effective_cycle_time)}s</span>
      </div>
      <div class="stat">
        <span class="stat-label">Inventory Cycles</span>
        <span class="stat-value">{formatNumber(item.inventory_cycles)}</span>
      </div>
    </div>

    <h3>Recipe Tree</h3>
    <RecipeTree tree={item.step_breakdown} />
  </div>
{:else}
  <div class="container">
    <p>Item not found or data is still loading.</p>
  </div>
{/if}
