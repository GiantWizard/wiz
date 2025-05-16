<script lang="ts">
  import { onMount } from 'svelte';
  import type { NewTopLevelItem } from '$lib/utils/typesAndTransforms';
  import { toTitleCase, abbreviateNumber } from '$lib/utils/typesAndTransforms';

  interface DisplayGridItem {
    id: string;
    name: string;
    profit_per_hour: number;
    crafting_cost_per_item: number;
    sell_price_per_item: number;
    cycles_per_hour: number;
    depth: number;
    crafting_savings_per_item: number;
    percent_flip: number;
  }

  let displayItems: DisplayGridItem[] = [];
  let loading: boolean = true;
  let error: string | null = null;

  const fetchBazaarData = async (): Promise<void> => {
    try {
      const response = await fetch('/optimizer_results.json'); 
      if (!response.ok) {
        throw new Error(`Bazaar Data Error: ${response.statusText} (${response.status})`);
      }
      
      const jsonData: { summary: any; results: NewTopLevelItem[] } = await response.json(); 

      if (!jsonData || !Array.isArray(jsonData.results)) { 
        console.error("Fetched data does not contain a 'results' array or is malformed:", jsonData);
        throw new Error("Expected an object with a 'results' array from the API.");
      }
      
      const rawData: NewTopLevelItem[] = jsonData.results; 

      displayItems = rawData
        .filter(item => item.calculation_possible) 
        .map(item => {
          const itemName = item.item_name || "UNKNOWN_ITEM"; 

          const mfQuantity = item.max_feasible_quantity > 0 ? item.max_feasible_quantity : 1;
          
          const crafting_cost_per_item = item.cost_at_optimal_qty / mfQuantity;
          const sell_price_per_item = item.revenue_at_optimal_qty / mfQuantity;
          const profit_per_item = item.max_profit / mfQuantity; 

          let profit_per_hour = 0;
          if (item.total_cycle_time_at_optimal_qty > 0) {
              profit_per_hour = (item.max_profit / (item.total_cycle_time_at_optimal_qty / 3600));
          }
          
          let items_per_hour = 0;
          if (item.total_cycle_time_at_optimal_qty > 0 && item.max_feasible_quantity > 0) {
              items_per_hour = (item.max_feasible_quantity / (item.total_cycle_time_at_optimal_qty / 3600));
          }

          let percent_flip = 0;
          if (crafting_cost_per_item > 0) { 
              percent_flip = (profit_per_item / crafting_cost_per_item) * 100;
          }

          return {
            id: itemName.trim(), 
            name: toTitleCase(itemName),
            profit_per_hour: profit_per_hour,
            crafting_cost_per_item: crafting_cost_per_item,
            sell_price_per_item: sell_price_per_item,
            cycles_per_hour: items_per_hour, 
            depth: item.recipe_tree?.max_sub_tree_depth ?? item.max_recipe_depth ?? 0,
            crafting_savings_per_item: profit_per_item,
            percent_flip: percent_flip,
          };
        }).sort((a, b) => b.profit_per_hour - a.profit_per_hour); 

    } catch (err: unknown) {
      error = err instanceof Error ? `Bazaar Error: ${err.message}` : 'Bazaar Error: Unknown error fetching or processing data.';
      console.error("Error in fetchBazaarData:", error); 
    } finally {
      loading = false;
    }
  };

  onMount(fetchBazaarData);

  const flipColor = (pct: number): string => {
    if (pct > 10) return 'text-green-400'; 
    if (pct > 2) return 'text-green-500';
    if (pct < -10) return 'text-red-400'; 
    if (pct < -2) return 'text-red-500';
    return 'text-gray-400'; 
  };

  function handleImageError(event: Event) {
    const imgElement = event.target as HTMLImageElement;
    console.error('IMAGE LOAD FAILED:', imgElement.src, 'Natural Width:', imgElement.naturalWidth);
  }
</script>

<style>
  .container { 
    padding: 1rem; 
    max-width: 1536px; 
    margin-left: auto;
    margin-right: auto;
  }
  .grid { 
    display: grid; 
    gap: 1rem; 
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); 
  }
  .card { 
    background-color: #1f2937; 
    border-radius: 0.75rem; 
    padding: 1rem; 
    box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1), 0 2px 4px -1px rgba(0,0,0,0.06); 
    transition-property: transform, box-shadow;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 200ms;
    text-decoration: none; 
    color: inherit; 
    display: flex; 
    flex-direction: column; 
    justify-content: space-between; 
  }
  .card:hover { 
    transform: translateY(-0.25rem); 
    box-shadow: 0 10px 15px -3px rgba(0,0,0,0.1), 0 4px 6px -2px rgba(0,0,0,0.05); 
  }
  .image-wrapper { 
    margin-bottom: 1rem; 
    display: flex; 
    justify-content: center; 
    align-items: center; 
    height: 6rem; 
  } 
  .image-wrapper img { 
    max-width: 100%; 
    max-height: 100%; 
    object-fit: contain; 
    border-radius: 0.375rem; 
  } 
  .item-info { 
    text-align: center; 
    margin-bottom: 1rem; 
  }
  .item-name { 
    margin-bottom: 0.25rem; 
    font-size: 1.125rem; 
    font-weight: 600; 
    color: #f3f4f6; 
    line-height: 1.4;
    min-height: 2.8em; 
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .price { 
    margin-bottom: 0.5rem; 
    font-size: 1.25rem; 
    font-weight: 700; 
    color: #a78bfa; 
  } 
  .stats { 
    display: flex; 
    flex-direction: column; 
    gap: 0.5rem; 
  }
  .stats-line { 
    display: flex; 
    justify-content: space-between; 
    gap: 0.5rem; 
  }
  .stat { 
    display: flex; 
    flex-direction: column; 
    flex: 1; 
    background-color: #374151; 
    border-radius: 0.375rem; 
    text-align: center; 
    padding: 0.5rem 0.25rem; 
  }
  .stat-value { 
    font-size: 0.875rem; 
    font-weight: 600; 
    color: #e5e7eb; 
  } 
  .stat-sub { 
    font-size: 0.625rem; 
    color: #9ca3af; 
    margin-top: 0.125rem; 
    text-transform: uppercase;
  } 
  .sharp-image { 
    image-rendering: -moz-crisp-edges; 
    image-rendering: -webkit-crisp-edges; 
    image-rendering: pixelated; 
    image-rendering: crisp-edges; 
  }
</style>

<div class="container">
  {#if loading}
    <div class="text-center text-xl text-gray-400 py-20">
      <svg class="animate-spin h-8 w-8 text-purple-400 mx-auto mb-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
      Loading Profitable Bazaar Items...
    </div>
  {:else if error}
    <div class="text-center text-xl text-red-500 bg-red-900 bg-opacity-30 p-10 rounded-lg">
      <h3 class="font-semibold mb-2">❌ Error Loading Data</h3>
      <p class="text-sm text-red-400">{error}</p>
      <p class="text-xs text-gray-500 mt-3">Please ensure the data source at /optimizer_results.json is available and correctly formatted.</p>
    </div>
  {:else if displayItems.length === 0}
    <div class="text-center text-xl text-gray-400 py-20">
      <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="mx-auto mb-3 text-gray-500 lucide lucide-search-x"><path d="m13.5 8.5-5 5"/><path d="m8.5 8.5 5 5"/><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
      No bazaar items found or calculable at the moment.
    </div>
  {:else}
    <div class="grid">
      {#each displayItems as dItem (dItem.id)}
        {@const imgSrc = `https://sky.coflnet.com/static/icon/${dItem.id}`}
        <!-- Rigorous Logging -->
        {console.log(`Item ID for image: '${dItem.id}', Type: ${typeof dItem.id}, Length: ${dItem.id?.length}`)}
        {console.log('Final imgSrc:', imgSrc)}

        <a class="card" href={`/bazaaritems/${encodeURIComponent(dItem.id)}`}>
          <div class="image-wrapper">
            <img 
              src={imgSrc}
              alt="" 
              class="sharp-image"
              loading="lazy"
              width="64" height="64" 
              on:error={handleImageError}
            />
            <!-- Comment correctly placed -->
          </div>
          <div class="item-info">
            <div class="item-name">{dItem.name}</div>
            <div class="price">{abbreviateNumber(dItem.profit_per_hour, dItem.profit_per_hour === 0 ? 0 : 1)}/h</div>
          </div>
          <div class="stats">
            <div class="stats-line">
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(dItem.crafting_cost_per_item)}</span>
                <span class="stat-sub">Craft Cost</span>
              </div>
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(dItem.sell_price_per_item)}</span>
                <span class="stat-sub">Sell Price</span>
              </div>
            </div>
            <div class="stats-line">
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(dItem.cycles_per_hour, 0)}</span>
                <span class="stat-sub">Items/h</span>
              </div>
              <div class="stat">
                <span class="stat-value">{dItem.depth}</span>
                <span class="stat-sub">Depth</span>
              </div>
            </div>
            <div class="stats-line">
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(dItem.crafting_savings_per_item)}</span>
                <span class="stat-sub">Profit/Item</span>
              </div>
              <div class="stat">
                <span class={`stat-value ${flipColor(dItem.percent_flip)}`}>
                  {dItem.percent_flip.toFixed(1)}%
                </span>
                <span class="stat-sub">Flip</span>
              </div>
            </div>
          </div>
        </a>
      {/each}
    </div>
  {/if}
</div>