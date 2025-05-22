<script lang="ts">
  // No longer need onMount for fetching here
  import type { KoyebOptimizedItemResult, KoyebOptimizationSummary, NewTopLevelItem } from '$lib/types/koyeb'; // Ensure NewTopLevelItem is correctly typed or part of KoyebOptimizedItemResult
  import { toTitleCase, abbreviateNumber } from '$lib/utils/typesAndTransforms'; // Assuming these utils exist and are typed

  interface DisplayGridItem {
    id: string; // Typically the Hypixel Item ID, e.g., "ENCHANTED_LAVA_BUCKET"
    name: string; // User-friendly display name
    profit_per_hour: number;
    crafting_cost_per_item: number;
    sell_price_per_item: number;
    cycles_per_hour: number; // How many full craft-to-sell cycles per hour
    depth: number; // Max recipe depth
    crafting_savings_per_item: number; // Essentially profit_per_item
    percent_flip: number;
  }

  // --- Props received from +page.svelte ---
  export let optimizerResults: KoyebOptimizedItemResult[] = [];
  export let optimizerSummary: KoyebOptimizationSummary | undefined = undefined;
  // The parent component (+page.svelte) will handle the primary loading and fetch error states.
  // This component will just display what it's given or its own transformation errors.

  let displayItems: DisplayGridItem[] = [];
  let transformationError: string | null = null; // For errors specific to data transformation here
  
  // Reactive statement to transform data when props change
  $: {
    transformationError = null; // Reset transformation error on new data
    if (optimizerResults && optimizerResults.length > 0) {
      try {
        displayItems = optimizerResults
          .filter(item => item.calculation_possible) // Only process items where calculation was possible
          .map(item => {
            // IMPORTANT: Ensure KoyebOptimizedItemResult contains all fields accessed below.
            // If NewTopLevelItem is a more detailed type that KoyebOptimizedItemResult should conform to,
            // this cast might be okay, but it's better if KoyebOptimizedItemResult is already comprehensive.
            const rawItem = item as unknown as NewTopLevelItem; 

            const itemName = rawItem.item_name || "UNKNOWN_ITEM";
            // Use max_feasible_quantity for calculations, default to 1 to avoid division by zero if it's 0 or undefined.
            const mfQuantity = (rawItem.max_feasible_quantity != null && rawItem.max_feasible_quantity > 0) 
                                ? rawItem.max_feasible_quantity 
                                : 1;
            
            const crafting_cost_per_item = rawItem.cost_at_optimal_qty / mfQuantity;
            const sell_price_per_item = rawItem.revenue_at_optimal_qty / mfQuantity;
            const profit_per_item = rawItem.max_profit / mfQuantity; // This is essentially crafting_savings_per_item

            let profit_per_hour = 0;
            if (rawItem.total_cycle_time_at_optimal_qty != null && rawItem.total_cycle_time_at_optimal_qty > 0) {
                profit_per_hour = (rawItem.max_profit / (rawItem.total_cycle_time_at_optimal_qty / 3600));
            }
            
            let cycles_per_hour = 0;
            if (rawItem.total_cycle_time_at_optimal_qty != null && rawItem.total_cycle_time_at_optimal_qty > 0 && 
                rawItem.max_feasible_quantity != null && rawItem.max_feasible_quantity > 0) {
                cycles_per_hour = (rawItem.max_feasible_quantity / (rawItem.total_cycle_time_at_optimal_qty / 3600));
            }

            let percent_flip = 0;
            if (crafting_cost_per_item > 0 && isFinite(crafting_cost_per_item)) { 
                percent_flip = (profit_per_item / crafting_cost_per_item) * 100;
            } else if (profit_per_item > 0 && crafting_cost_per_item === 0) { // Profit from a free item
                percent_flip = Infinity; 
            } // else it remains 0 if no profit or invalid cost


            return {
              id: itemName.trim().toUpperCase(), // Standardize ID for consistency (e.g., for image URLs)
              name: toTitleCase(itemName.replace(/_/g, " ")), // Format name for display
              profit_per_hour: isFinite(profit_per_hour) ? profit_per_hour : 0,
              crafting_cost_per_item: isFinite(crafting_cost_per_item) ? crafting_cost_per_item : 0,
              sell_price_per_item: isFinite(sell_price_per_item) ? sell_price_per_item : 0,
              cycles_per_hour: isFinite(cycles_per_hour) ? cycles_per_hour : 0,
              depth: rawItem.recipe_tree?.max_sub_tree_depth ?? rawItem.max_recipe_depth ?? 0,
              crafting_savings_per_item: isFinite(profit_per_item) ? profit_per_item : 0,
              percent_flip: percent_flip, // Can be Infinity
            };
          }).sort((a, b) => b.profit_per_hour - a.profit_per_hour); // Sort by profit per hour descending
      } catch (err) {
        transformationError = err instanceof Error ? `Data Transformation Error: ${err.message}` : 'Unknown error transforming bazaar item data.';
        console.error("Error in BazaarProfitableItems data transformation:", transformationError, optimizerResults);
        displayItems = []; // Clear items on transformation error to avoid displaying partial/corrupt data
      }
    } else {
      displayItems = []; // Clear if no optimizerResults are passed or if it's empty
    }
  }

  const flipColor = (pct: number): string => {
    if (!isFinite(pct)) return 'text-yellow-400 font-bold'; // Special color for "INF%"
    if (pct > 25) return 'text-green-400'; 
    if (pct > 5) return 'text-green-500';
    if (pct < -25) return 'text-red-400'; 
    if (pct < -5) return 'text-red-500';
    return 'text-gray-400'; 
  };

  function handleImageError(event: Event) {
    const imgElement = event.target as HTMLImageElement;
    console.warn('IMAGE LOAD FAILED for item ID:', imgElement.alt, 'Attempted SRC:', imgElement.src);
    // You could set a default placeholder image if an item's icon fails to load
    // imgElement.src = '/path/to/default-placeholder-icon.png';
    imgElement.style.visibility = 'hidden'; // Hide broken image icon, or apply a class
  }
</script>

<style>
  .container { 
    padding: 1rem; 
    max-width: 1536px; /* Corresponds to Tailwind's 2xl, or adjust as needed */
    margin-left: auto;
    margin-right: auto;
  }
  .grid { 
    display: grid; 
    gap: 1rem; /* Tailwind: gap-4 */
    /* Responsive grid columns */
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); /* Default for smaller screens */
  }
  @media (min-width: 640px) { /* sm */
    .grid { grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); }
  }
  @media (min-width: 1024px) { /* lg */
    .grid { grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); }
  }

  .card { 
    background-color: #1f2937; /* Tailwind: bg-gray-800 */
    border-radius: 0.75rem; /* Tailwind: rounded-xl */
    padding: 1rem; /* Tailwind: p-4 */
    box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1), 0 2px 4px -1px rgba(0,0,0,0.06); /* Tailwind: shadow-lg */
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
    transform: translateY(-0.25rem); /* Tailwind: hover:-translate-y-1 */
    box-shadow: 0 10px 15px -3px rgba(0,0,0,0.1), 0 4px 6px -2px rgba(0,0,0,0.05); /* Tailwind: hover:shadow-xl */
  }
  .image-wrapper { 
    margin-bottom: 1rem; /* Tailwind: mb-4 */
    display: flex; 
    justify-content: center; 
    align-items: center; 
    height: 6rem; /* Tailwind: h-24 */
  } 
  .image-wrapper img { 
    max-width: 100%; 
    max-height: 100%; 
    object-fit: contain; 
    border-radius: 0.375rem; /* Tailwind: rounded-md */
  } 
  .item-info { 
    text-align: center; 
    margin-bottom: 1rem; /* Tailwind: mb-4 */
  }
  .item-name { 
    margin-bottom: 0.25rem; /* Tailwind: mb-1 */
    font-size: 1.125rem; /* Tailwind: text-lg */
    font-weight: 600; /* Tailwind: font-semibold */
    color: #f3f4f6; /* Tailwind: text-gray-100 */
    line-height: 1.4;
    min-height: 2.8em; /* Approximately 2 lines of text, adjust as needed */
    display: flex; /* For vertical centering if needed */
    align-items: center;
    justify-content: center;
  }
  .price { 
    margin-bottom: 0.5rem; /* Tailwind: mb-2 */
    font-size: 1.25rem; /* Tailwind: text-xl */
    font-weight: 700; /* Tailwind: font-bold */
    color: #a78bfa; /* Your primary accent color */
  } 
  .stats { 
    display: flex; 
    flex-direction: column; 
    gap: 0.5rem; /* Tailwind: gap-2 */
  }
  .stats-line { 
    display: flex; 
    justify-content: space-between; 
    gap: 0.5rem; /* Tailwind: gap-2 */
  }
  .stat { 
    display: flex; 
    flex-direction: column; 
    flex: 1; /* Grow to fill space */
    background-color: #374151; /* Tailwind: bg-gray-700 */
    border-radius: 0.375rem; /* Tailwind: rounded-md */
    text-align: center; 
    padding: 0.5rem 0.25rem; /* Tailwind: py-2 px-1 */
  }
  .stat-value { 
    font-size: 0.875rem; /* Tailwind: text-sm */
    font-weight: 600; /* Tailwind: font-semibold */
    color: #e5e7eb; /* Tailwind: text-gray-200 */
  } 
  .stat-sub { 
    font-size: 0.625rem; /* ~10px */
    color: #9ca3af; /* Tailwind: text-gray-400 */
    margin-top: 0.125rem; /* Tailwind: mt-0.5 */
    text-transform: uppercase;
  } 
  .sharp-image { 
    image-rendering: -moz-crisp-edges; 
    image-rendering: -webkit-crisp-edges; 
    image-rendering: pixelated; 
    image-rendering: crisp-edges; 
  }
  .no-items-message, .error-message {
    text-align: center;
    padding: 5rem 1rem; /* Tailwind: py-20 px-4 */
    font-size: 1.25rem; /* Tailwind: text-xl */
    color: #9ca3af; /* Tailwind: text-gray-400 */
  }
  .error-message {
    color: #f87171; /* Tailwind: text-red-400 */
    background-color: rgba(127, 29, 29, 0.3); /* Tailwind: bg-red-900 bg-opacity-30 */
    border-radius: 0.5rem; /* Tailwind: rounded-lg */
    padding: 2.5rem; /* Tailwind: p-10 */
  }
  .error-message h3 {
    font-weight: 600; /* Tailwind: font-semibold */
    margin-bottom: 0.5rem; /* Tailwind: mb-2 */
  }
  .error-message .details {
    font-size: 0.875rem; /* Tailwind: text-sm */
    color: #fca5a5; /* Tailwind: text-red-300 */
  }
</style>

<div class="container">
  {#if transformationError}
    <div class="error-message">
      <h3>Internal Data Error</h3>
      <p class="details">{transformationError}</p>
    </div>
  {:else if displayItems.length > 0}
    {#if optimizerSummary}
      <div class="mb-8 p-4 bg-darker rounded-lg border border-secondary/20 text-sm">
        <h3 class="text-xl font-semibold text-primary mb-2">Run Summary</h3>
        <p class="text-light opacity-80">
          Last run: {new Date(optimizerSummary.run_timestamp).toLocaleString()}
        </p>
        {#if optimizerSummary.api_last_updated_timestamp}
          <p class="text-light opacity-70">
            API Data as of: {new Date(optimizerSummary.api_last_updated_timestamp).toLocaleString()}
          </p>
        {/if}
        <p class="text-light opacity-80">
          Items Calculated: {optimizerSummary.items_successfully_calculated} / {optimizerSummary.total_items_considered}
        </p>
        {#if optimizerSummary.items_with_calculation_errors > 0}
          <p class="text-red-400">
            Calculation Errors: {optimizerSummary.items_with_calculation_errors}
            <!-- Consider linking to /api/failed_items_report if you make a page for it -->
          </p>
        {/if}
      </div>
    {/if}

    <div class="grid">
      {#each displayItems as dItem (dItem.id)}
        {@const imgSrc = `https://sky.coflnet.com/static/icon/${dItem.id}`}
        <a class="card" href={`/bazaaritems/${encodeURIComponent(dItem.id)}`} title={`View details for ${dItem.name}`}>
          <div class="image-wrapper">
            <img 
              src={imgSrc}
              alt={dItem.name} 
              class="sharp-image"
              loading="lazy"
              width="64" height="64" 
              on:error={handleImageError}
            />
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
                  {isFinite(dItem.percent_flip) ? dItem.percent_flip.toFixed(1) + '%' : 'INF%'}
                </span>
                <span class="stat-sub">Flip</span>
              </div>
            </div>
          </div>
        </a>
      {/each}
    </div>
  {:else if !optimizerSummary && optimizerResults.length === 0}
    <!-- This case is covered by the parent's loading/error state for the initial fetch -->
    <!-- If optimizerResults is explicitly passed as empty by parent, and no summary, show this: -->
    <div class="no-items-message">
        <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="mx-auto mb-3 text-gray-500 lucide lucide-search-x"><path d="m13.5 8.5-5 5"/><path d="m8.5 8.5 5 5"/><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
        No items available in the data provided.
    </div>
  {:else if optimizerSummary && optimizerResults.filter(item => item.calculation_possible).length === 0}
    <!-- Data and summary received, but all items were filtered out (e.g., all had calculation_possible: false) -->
     {#if optimizerSummary}
      <div class="mb-8 p-4 bg-darker rounded-lg border border-secondary/20 text-sm">
        <h3 class="text-xl font-semibold text-primary mb-2">Run Summary</h3>
        <p class="text-light opacity-80">
          Last run: {new Date(optimizerSummary.run_timestamp).toLocaleString()}
        </p>
        {#if optimizerSummary.api_last_updated_timestamp}
          <p class="text-light opacity-70">
            API Data as of: {new Date(optimizerSummary.api_last_updated_timestamp).toLocaleString()}
          </p>
        {/if}
        <p class="text-light opacity-80">
          Items Calculated: {optimizerSummary.items_successfully_calculated} / {optimizerSummary.total_items_considered}
        </p>
        {#if optimizerSummary.items_with_calculation_errors > 0}
          <p class="text-red-400">
            Calculation Errors: {optimizerSummary.items_with_calculation_errors}
          </p>
        {/if}
      </div>
    {/if}
    <div class="no-items-message">
        <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="mx-auto mb-3 text-gray-500 lucide lucide-info"><circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="16" y2="12"/><line x1="12" x2="12.01" y1="8" y2="8"/></svg>
        No items meet display criteria (e.g., all items had calculation issues or no profit).
    </div>
  {/if}
  <!-- The main loading/error state for the initial fetch is handled by the parent +page.svelte -->
</div>