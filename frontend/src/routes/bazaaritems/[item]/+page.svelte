<script lang="ts">
  import { onMount } from 'svelte';
  import RecipeTree from '$lib/components/RecipeTree.svelte';
  import AveragePriceChart from '$lib/components/AveragePriceChart.svelte';

  // Types are for the `data.item` prop passed from the load function in +page.ts
  import type { 
      NewTopLevelItem, 
      TransformedTree, 
      RawMaterialReport,
      AvgPriceData as ChartAvgPriceData,
      PriceHistory
  } from '$lib/utils/typesAndTransforms';
  // Helpers are for display formatting within this component
  import { 
      simplifiedTransform, 
      getRawMaterialsFromNewTree,
      toTitleCase,
      abbreviateNumber,
      formatNumberSimple
  } from '$lib/utils/typesAndTransforms';

  export let data: { item: NewTopLevelItem }; // Data comes from +page.ts load function
  
  let item: NewTopLevelItem;
  $: item = data.item; // Reactive assignment

  // Component-specific reactive calculations
  let transformedRecipeTree: TransformedTree | undefined;
  let rawMaterialsList: RawMaterialReport[] = [];

  let displayProfitPerHour: number = 0;
  let displayCraftCostPerItem: number = 0;
  let displaySellPricePerItem: number = 0;
  let displayItemsPerHour: number = 0; 
  let displayProfitPerItem: number = 0;
  let displayPercentFlip: number = 0;
  let displayEffectiveCycleTimePerItem: number = 0; 
  let displayAcquisitionTimePerItem: number = 0;
  let displaySaleTimePerItem: number = 0;

  $: if (item) { 
    const mfQuantity = item.max_feasible_quantity > 0 ? item.max_feasible_quantity : 1;
    
    displayCraftCostPerItem = (item.cost_at_optimal_qty ?? 0) / mfQuantity;
    displaySellPricePerItem = (item.revenue_at_optimal_qty ?? 0) / mfQuantity;
    displayProfitPerItem = (item.max_profit ?? 0) / mfQuantity;
    displayAcquisitionTimePerItem = (item.acquisition_time_at_optimal_qty ?? 0) / mfQuantity;
    displaySaleTimePerItem = (item.sale_time_at_optimal_qty ?? 0) / mfQuantity;

    if ((item.total_cycle_time_at_optimal_qty ?? 0) > 0) {
        displayProfitPerHour = ((item.max_profit ?? 0) / ((item.total_cycle_time_at_optimal_qty ?? 1) / 3600));
        displayEffectiveCycleTimePerItem = (item.total_cycle_time_at_optimal_qty ?? 0) / mfQuantity;
    } else {
        displayProfitPerHour = 0;
        displayEffectiveCycleTimePerItem = 0;
    }
    
    if ((item.total_cycle_time_at_optimal_qty ?? 0) > 0 && item.max_feasible_quantity > 0) {
        displayItemsPerHour = (item.max_feasible_quantity / ((item.total_cycle_time_at_optimal_qty ?? 1) / 3600));
    } else {
        displayItemsPerHour = 0;
    }

    if (displayCraftCostPerItem > 0) {
        displayPercentFlip = (displayProfitPerItem / displayCraftCostPerItem) * 100;
    } else {
        displayPercentFlip = 0;
    }

    if (item.recipe_tree) {
        transformedRecipeTree = simplifiedTransform(item.recipe_tree);
        rawMaterialsList = getRawMaterialsFromNewTree(item.recipe_tree);
    } else {
        console.warn(`Item ${item.item_name} is missing recipe_tree (in +page.svelte script block).`);
        transformedRecipeTree = undefined;
        rawMaterialsList = [];
    }
  }
  
  // --- Price Data Processing for Charts ---
  let allAvgPrices: ChartAvgPriceData[] | null = null;
  let mainItemChartData: ChartAvgPriceData | null = null;

  // Interface for 3-day data (if not already in typesAndTransforms.ts)
  interface ThreeDayPrice { price: number; count: number; sales: number; clean_price?: number; clean_sales?: number; }
  interface ThreeDayData { [key: string]: ThreeDayPrice; }
  let all3DayData: ThreeDayData | null = null;

  let chartDataLoading = true;

  onMount(async () => {
    if (!item || !item.item_name) { 
        chartDataLoading = false;
        return;
    }
    console.log(`[DETAIL PAGE ONMOUNT for ${item.item_name}] Fetching chart data...`);
    try {
      const localResPromise = fetch('/avgPrices.json').catch(e => { console.error("avgPrices.json fetch error in onMount:", e); return null; });
      const threeDayResPromise = fetch('/3day.json').catch(e => { console.error("3day.json fetch error in onMount:", e); return null; });

      const [localRes, threeDayRes] = await Promise.all([localResPromise, threeDayResPromise]);

      if (localRes && localRes.ok) {
          allAvgPrices = await localRes.json();
      } else if(localRes) {
          console.error('Failed to fetch avgPrices.json in onMount', localRes.status, await localRes.text().catch(()=>"Could not read error body for avgPrices"));
      } else {
          console.error('localResPromise for avgPrices.json failed or returned null.');
      }

      if (threeDayRes && threeDayRes.ok) {
          all3DayData = await threeDayRes.json();
      } else if (threeDayRes) {
          console.error('Failed to fetch 3day.json in onMount', threeDayRes.status, await threeDayRes.text().catch(()=>"Could not read error body for 3day"));
      } else {
          console.error('threeDayResPromise for 3day.json failed or returned null.');
      }
    } catch (err) {
        console.error("Error in onMount chart data fetching logic:", err);
    } finally {
        chartDataLoading = false; 
    }
  });

  $: if (!chartDataLoading && item && item.item_name) { 
      mainItemChartData = findChartDataForDisplay(item.item_name);
  }

  function toSinglePointAvgData(itemName: string, price: number): ChartAvgPriceData {
    return {
      item: itemName,
      history: [{ buy: price, sell: price, timestamp: new Date().toISOString() }]
    };
  }

  function findChartDataForDisplay(itemName?: string): ChartAvgPriceData | null {
    if (!itemName) return null;
    if (allAvgPrices) {
      const localItem = allAvgPrices.find(d => d.item === itemName);
      if (localItem) return localItem;
    }
    if (all3DayData && all3DayData[itemName]) {
      const price = all3DayData[itemName].price;
      if (price != null) return toSinglePointAvgData(itemName, price);
    }
    console.log(`[Chart Data] No data found for ${itemName}`);
    return null;
  }

  function handleImageErrorLocal(event: Event) {
    const imgElement = event.target as HTMLImageElement;
    console.error('IMAGE LOAD FAILED (Detail Page Component):', imgElement.src, 'Natural Width:', imgElement.naturalWidth);
  }
</script>

{#if item && item.item_name} <!-- Check item and item_name before trying to render -->
  {#if item.calculation_possible}
    <div class="p-4 md:p-8 max-w-4xl mx-auto bg-darker rounded-lg shadow-xl space-y-12 md:space-y-16">

      <section id="stats-section" class="space-y-8 md:space-y-10">
        <div class="text-center space-y-2">
          <h1 class="text-3xl md:text-4xl font-bold text-light font-inter">
            {toTitleCase(item.item_name)}
          </h1>
          <h2 class="text-2xl md:text-3xl font-semibold text-accent">
            {abbreviateNumber(displayProfitPerHour)}/h
          </h2>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-6 md:gap-8">
          <div class="space-y-3 md:space-y-4 bg-dark p-4 rounded-lg shadow-md">
            <h3 class="text-xl md:text-2xl font-semibold text-accent mb-3">Profit & Efficiency</h3>
            {@html [
              { label: "Craft Cost/Item", value: abbreviateNumber(displayCraftCostPerItem) },
              { label: "Sell Price/Item", value: abbreviateNumber(displaySellPricePerItem) },
              { label: "Profit/Item", value: `▲ ${abbreviateNumber(displayProfitPerItem)}` },
              { label: "Items/Hour", value: abbreviateNumber(displayItemsPerHour) },
              { label: "% Flip", value: `${displayPercentFlip.toFixed(1)}%`, highlight: true }
            ].map(stat => (
              `<div class="flex justify-between border-b border-gray-700 pb-1.5 pt-1">` +
              `<span class="text-sm md:text-base text-gray-400">${stat.label}</span>` +
              `<span class="text-sm md:text-base font-medium text-light ${stat.highlight ? 'bg-primary px-2 py-0.5 rounded-md' : ''}">${stat.value}</span>` +
              `</div>`
            )).reduce((html, current) => html + current, '')}
          </div>

          <div class="space-y-3 md:space-y-4 bg-dark p-4 rounded-lg shadow-md">
            <h3 class="text-xl md:text-2xl font-semibold text-accent mb-3">Timing & Depth</h3>
            {@html [
              { label: "Max Recipe Depth", value: item.recipe_tree?.max_sub_tree_depth ?? item.max_recipe_depth ?? 0 },
              { label: "Acquisition Time/Item", value: `${formatNumberSimple(displayAcquisitionTimePerItem, 2)}s` },
              { label: "Sale Time/Item", value: `${formatNumberSimple(displaySaleTimePerItem, 2)}s` },
              { label: "Effective Cycle Time/Item", value: `${formatNumberSimple(displayEffectiveCycleTimePerItem, 2)}s` },
              { label: "Max Batch Size", value: abbreviateNumber(item.max_feasible_quantity, 0) }
            ].map(stat => (
              `<div class="flex justify-between border-b border-gray-700 pb-1.5 pt-1">` +
              `<span class="text-sm md:text-base text-gray-400">${stat.label}</span>` +
              `<span class="text-sm md:text-base font-medium text-light">${stat.value}</span>` +
              `</div>`
            )).reduce((html, current) => html + current, '')}
          </div>
        </div>
      </section>

      {#if transformedRecipeTree && transformedRecipeTree.item }
      <section id="recipe-tree-section">
        <h2 class="text-center text-2xl md:text-3xl font-bold text-light mb-4">
            Recipe for 1 {toTitleCase(transformedRecipeTree.item)}
        </h2>
        <div class="relative bg-dark p-4 md:p-6 rounded-lg shadow-inner overflow-x-auto">
          <RecipeTree tree={transformedRecipeTree} parentQuantity={1} isTopLevel={true} />
        </div>
      </section>
      {/if}
      
      <section id="item-graphs-section">
        <h2 class="text-center text-2xl md:text-3xl font-bold text-light mb-6 md:mb-8">Price Graphs</h2>
        
        <div id="main-item-graph" class="mb-8 md:mb-12 bg-dark p-4 rounded-lg shadow-md">
          <div class="flex items-center justify-center mb-3 space-x-2">
            <img
              src={`https://sky.coflnet.com/static/icon/${item.item_name}`}
              alt={toTitleCase(item.item_name)}
              class="w-8 h-8 md:w-10 md:h-10 sharp-image rounded-sm"
              loading="lazy"
              on:error={handleImageErrorLocal}
            />
            <span class="text-xl md:text-2xl font-semibold text-light">{toTitleCase(item.item_name)}</span>
          </div>
          {#if chartDataLoading}
            <p class="text-light text-center text-sm py-10">📈 Loading price data...</p>
          {:else if mainItemChartData}
            <AveragePriceChart avgData={mainItemChartData} width={600} height={280} padding={60} />
          {:else}
            <p class="text-light text-center text-sm py-10">📊 No average price data available for this item.</p>
          {/if}
        </div>
        
        {#if rawMaterialsList.length > 0}
          <h3 class="text-center text-xl md:text-2xl font-semibold text-accent mb-6">Raw Material Graphs</h3>
          <div id="sub-item-graphs" class="grid grid-cols-1 md:grid-cols-2 gap-6 md:gap-8">
            {#each rawMaterialsList as mat (mat.ingredient)}
              {@const chartData = findChartDataForDisplay(mat.ingredient)}
              <div class="bg-dark p-4 rounded-lg shadow-md">
                <div class="flex items-center justify-center mb-2 space-x-2">
                  <img
                    src={`https://sky.coflnet.com/static/icon/${mat.ingredient}`}
                    alt={toTitleCase(mat.ingredient)}
                    class="w-6 h-6 sharp-image rounded-sm"
                    loading="lazy"
                    on:error={handleImageErrorLocal}
                  />
                  <span class="text-light font-medium">{toTitleCase(mat.ingredient)}</span>
                </div>
                {#if chartDataLoading && !chartData} 
                     <p class="text-sm text-light text-center mt-2 py-8">📈 Loading price data...</p>
                {:else if chartData}
                  <AveragePriceChart avgData={chartData} width={300} height={220} padding={50} />
                {:else}
                  <p class="text-sm text-light text-center mt-2 py-8">📊 No data for {toTitleCase(mat.ingredient)}</p>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </section>
      
    </div>
  {:else if item && !item.calculation_possible}
    <div class="p-8 max-w-3xl mx-auto bg-darker rounded-lg shadow-xl text-center">
      <h1 class="text-3xl font-bold text-light mb-4">{toTitleCase(item.item_name)}</h1>
      <p class="text-light text-lg">Profit calculation is not currently possible for this item.</p>
      <p class="text-sm text-gray-400 mt-2">This might be due to missing price data for essential components or extreme market volatility.</p>
    </div>
  {/if} <!-- Closes inner #if item.calculation_possible -->
{:else} 
  <div class="p-8 max-w-3xl mx-auto bg-darker rounded-lg shadow-lg">
    <p class="text-light text-center py-10">Item data is loading or item was not found. If testing with minimal load, ensure you are requesting SKELETON_KEY or expand the test cases in the load function.</p>
  </div>
{/if} <!-- Closes outer #if item && item.item_name -->

<style>
  #stats-section,
  #recipe-tree-section,
  #item-graphs-section {
    margin-bottom: 3rem; 
  }
  @media (min-width: 768px) { 
    #stats-section,
    #recipe-tree-section,
    #item-graphs-section {
      margin-bottom: 4rem;
    }
  }
  .sharp-image {
    image-rendering: -moz-crisp-edges;
    image-rendering: -webkit-crisp-edges;
    image-rendering: pixelated;
    image-rendering: crisp-edges;
  }
</style>