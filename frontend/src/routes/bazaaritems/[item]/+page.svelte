<script context="module" lang="ts">
  import type { PageLoad } from './$types';

  export const load: PageLoad = async ({ params, fetch }) => {
    const decodedItem = decodeURIComponent(params.item);
    const res = await fetch('/top_40_bazaar_crafts.json');
    if (!res.ok) {
      throw new Error('Could not fetch profitable items data');
    }
    const items = await res.json();
    if (!Array.isArray(items)) {
      throw new Error('Fetched JSON is not an array');
    }
    const foundItem = items.find(i => i.item === decodedItem);
    if (!foundItem) {
      throw new Error(`Item ${decodedItem} not found`);
    }
    return {
      props: {
        item: foundItem
      }
    };
  };
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import RecipeTree from '$lib/components/RecipeTree.svelte';
  import AveragePriceChart from '$lib/components/AveragePriceChart.svelte';
<<<<<<< HEAD

  // ================================
  // Helpers
  // ================================
  function formatLargeNumber(num: number): string {
    const abs = Math.abs(num);
    if (abs < 1000) {
      return num.toString();
    } else if (abs < 1_000_000) {
      return Math.round(num / 1_000) + 'k';
    } else if (abs < 1_000_000_000) {
      return Math.round(num / 1_000_000) + 'm';
    } else {
      return Math.round(num / 1_000_000_000) + 'b';
    }
  }

=======

  // Helpers
  function formatLargeNumber(num: number): string {
    const abs = Math.abs(num);
    if (abs < 1000) {
      return num.toString();
    } else if (abs < 1_000_000) {
      return Math.round(num / 1_000) + 'k';
    } else if (abs < 1_000_000_000) {
      return Math.round(num / 1_000_000) + 'm';
    } else {
      return Math.round(num / 1_000_000_000) + 'b';
    }
  }

>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  function toTitleCase(str: string): string {
    return str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (_, g1) => g1.toUpperCase());
  }

<<<<<<< HEAD
  // ================================
  // Interfaces
  // ================================
=======
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  interface Tree {
    item?: string;
    note?: string;
    ingredients?: Ingredient[];
  }

  interface Ingredient {
    ingredient: string;
    total_needed: number;
    buy_method?: string;
    cost_per_unit?: number;
    sub_breakdown?: Tree;
  }
<<<<<<< HEAD

  interface BazaarItem {
    item: string;
    profit_per_hour: number;
    crafting_cost: number;
    sell_price: number;
    cycles_per_hour: number;
    longest_step_count: number;
    crafting_savings: number;
    buy_fill_time: number;
    sell_fill_time: number;
    effective_cycle_time: number;
    inventory_cycles: number;
    step_breakdown: Tree;
  }

=======

  interface BazaarItem {
    item: string;
    profit_per_hour: number;
    crafting_cost: number;
    sell_price: number;
    cycles_per_hour: number;
    longest_step_count: number;
    crafting_savings: number;
    buy_fill_time: number;
    sell_fill_time: number;
    effective_cycle_time: number;
    inventory_cycles: number;
    step_breakdown: Tree;
  }

>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  // Percent flip helper
  function percentFlip(item: BazaarItem): number {
    if (!item.crafting_cost) return 0;
    return (item.crafting_savings / item.crafting_cost) * 100;
  }

<<<<<<< HEAD
  // ================================
  // Props and Data
  // ================================
=======
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  export let data: { item: BazaarItem };
  let item: BazaarItem;
  $: item = data.item;

<<<<<<< HEAD
  // ================================
  // Raw Materials Processing
  // ================================
=======
  // Gather raw materials
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  function getRawMaterials(tree: Tree, multiplier: number = 1): Ingredient[] {
    const rawMap: Record<string, Ingredient> = {};
    if (tree.ingredients) {
      tree.ingredients.forEach(ing => {
        const total = ing.total_needed * multiplier;
<<<<<<< HEAD
        // Recurse if a sub_breakdown exists with no note
=======
        // If there's a sub_breakdown with no note, recurse
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
        if (ing.sub_breakdown && !ing.sub_breakdown.note) {
          const subRaws = getRawMaterials(ing.sub_breakdown, total);
          subRaws.forEach(raw => {
            if (rawMap[raw.ingredient]) {
              rawMap[raw.ingredient].total_needed += raw.total_needed;
            } else {
              rawMap[raw.ingredient] = { ...raw };
            }
          });
        } else {
          // Otherwise, it's a raw material
          if (rawMap[ing.ingredient]) {
            rawMap[ing.ingredient].total_needed += total;
          } else {
            rawMap[ing.ingredient] = {
              ingredient: ing.ingredient,
              total_needed: total
            };
          }
        }
      });
    }
    return Object.values(rawMap);
  }

  let rawMaterials: Ingredient[] = [];
  $: if (item && item.step_breakdown) {
    rawMaterials = getRawMaterials(item.step_breakdown);
  }

<<<<<<< HEAD
  // ================================
  // Price Data Processing
  // ================================
=======
  // ---------- Local avgPrices (multiple data points) ----------
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  interface PriceRecord {
    buy: number;
    sell: number;
    timestamp: string;
  }
  interface AvgPriceData {
    item: string;
    history: PriceRecord[];
  }
  let allAvgPrices: AvgPriceData[] | null = null;
  let mainAvgData: AvgPriceData | null = null;

<<<<<<< HEAD
=======
  // ---------- 3day data (single price, no timestamps) ----------
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  interface ThreeDayData {
    [key: string]: {
      price: number;
      count: number;
      sales: number;
      clean_price?: number;
      clean_sales?: number;
    };
  }
  let all3DayData: ThreeDayData | null = null;

<<<<<<< HEAD
  // ================================
  // Data Fetching
  // ================================
  onMount(async () => {
    try {
      // Fetch local average prices
=======
  // On mount, fetch from local JSON and from /frontend/static/3day.json
  onMount(async () => {
    try {
      // 1) Local
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
      const localRes = await fetch('/avgPrices.json');
      if (!localRes.ok) {
        console.error('Failed to fetch avgPrices.json');
        return;
      }
      allAvgPrices = await localRes.json();
      if (Array.isArray(allAvgPrices)) {
        mainAvgData = allAvgPrices.find(d => d.item === item.item) || null;
      }

<<<<<<< HEAD
      // Fetch 3-day data
=======
      // 2) 3day
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
      const threeDayRes = await fetch('/3day.json');
      if (!threeDayRes.ok) {
        console.error('Failed to fetch 3day.json');
        return;
      }
      all3DayData = await threeDayRes.json();
    } catch (err) {
      console.error('Error fetching data:', err);
    }
  });

<<<<<<< HEAD
  // Convert a single 3day item into minimal AvgPriceData
=======
  // Convert a single 3day item into a minimal AvgPriceData
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
  function toSinglePointAvgData(itemName: string, price: number): AvgPriceData {
    return {
      item: itemName,
      history: [
        {
          buy: price,
          sell: price,
<<<<<<< HEAD
          timestamp: new Date().toISOString()
=======
          timestamp: new Date().toISOString() // fake single timestamp
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
        }
      ]
    };
  }

<<<<<<< HEAD
  // Find chart data from local data or fallback to 3day data
  function findChartData(itemName: string): AvgPriceData | null {
=======
  // Return either multi-point local data or single-point 3day data
  function findChartData(itemName: string): AvgPriceData | null {
    // Check local first
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
    if (allAvgPrices) {
      const localItem = allAvgPrices.find(d => d.item === itemName);
      if (localItem) return localItem;
    }
<<<<<<< HEAD
=======
    // If not found, check 3day
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
    if (all3DayData && all3DayData[itemName]) {
      const price = all3DayData[itemName].price;
      return toSinglePointAvgData(itemName, price);
    }
    return null;
  }
</script>

{#if item}
<<<<<<< HEAD
<div class="p-8 max-w-4xl mx-auto bg-darker rounded-lg shadow-lg space-y-16">

  <!-- ================================================== -->
  <!-- Section 1: Main Overview (Stats & Details) -->
  <!-- ================================================== -->
  <section id="stats-section" class="space-y-10">
    <!-- Stats Header -->
    <div class="text-center space-y-2">
      <h1 class="text-3xl md:text-4xl font-bold text-light font-inter">
        {toTitleCase(item.item)}
      </h1>
      <h2 class="text-2xl md:text-3xl font-semibold text-accent">
        {formatLargeNumber(item.profit_per_hour)}/h
      </h2>
=======
<div class="p-8 max-w-4xl mx-auto bg-darker rounded-lg shadow-lg space-y-10">
  <!-- 1) Stats Section -->
  <div class="text-center space-y-2">
    <div class="text-3xl md:text-4xl font-bold text-light font-inter">
      {toTitleCase(item.item)}
    </div>
    <div class="text-2xl md:text-3xl font-semibold text-accent">
      {formatLargeNumber(item.profit_per_hour)}/h
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
    </div>
  </div>

<<<<<<< HEAD
    <!-- Profit & Efficiency & Timing Details -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
      <!-- Profit & Efficiency Details -->
      <div class="space-y-4">
        <h3 class="text-xl md:text-2xl font-semibold text-accent">Profit &amp; Efficiency</h3>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Craft Cost</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.crafting_cost)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Sell Price</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.sell_price)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Cycles</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.cycles_per_hour)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Savings</span>
          <span class="text-sm md:text-base font-medium text-light">
            ▲ {formatLargeNumber(item.crafting_savings)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">% Flip</span>
          <span class="text-sm md:text-base font-medium text-light bg-primary px-2 py-1 rounded">
            {formatLargeNumber(percentFlip(item))}%
          </span>
        </div>
      </div>

      <!-- Timing & Depth Details -->
      <div class="space-y-4">
        <h3 class="text-xl md:text-2xl font-semibold text-accent">Timing &amp; Depth</h3>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Max Depth</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.longest_step_count)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Buy Fill Time</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.buy_fill_time)}s
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Sell Fill Time</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.sell_fill_time)}s
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Effective Cycle Time</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.effective_cycle_time)}s
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Inventory Cycles</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatLargeNumber(item.inventory_cycles)}
          </span>
        </div>
      </div>
    </div>
  </section>

  <!-- ================================================== -->
  <!-- Section 2: Recipe Tree -->
  <!-- ================================================== -->
  <section id="recipe-tree-section">
    <h2 class="text-center text-2xl md:text-3xl font-bold text-light mb-4">Recipe Tree</h2>
    <div class="relative bg-darker p-6 rounded-lg shadow-lg">
      <RecipeTree tree={item.step_breakdown} />
    </div>
  </section>

  <!-- ================================================== -->
  <!-- Section 3: Graphs -->
  <!-- ================================================== -->
  <section id="item-graphs-section">
    <h2 class="text-center text-3xl md:text-4xl font-bold text-light mb-4">Item Graphs</h2>
    
    <!-- Main Item Graph Block with Icon and Name -->
    <div id="main-item-graph" class="mb-6">
      <div class="flex items-center justify-center mb-2 space-x-2">
        <img
          src={"https://sky.shiiyu.moe/item/" + item.item}
          alt={item.item}
          class="w-8 h-8 md:w-12 md:h-12"
        />
        <span class="text-2xl md:text-3xl font-bold text-light">{toTitleCase(item.item)}</span>
      </div>
      {#if (mainAvgData !== null) || (all3DayData !== null)}
        {#if findChartData(item.item) !== null}
          <AveragePriceChart
            avgData={findChartData(item.item)}
            width={700}
            height={300}
            padding={50}
          />
        {:else}
          <p class="text-light text-center">
            No average price data available for this item.
          </p>
        {/if}
      {:else}
        <p class="text-light text-center">
          No average price data available for this item.
        </p>
      {/if}
    </div>
    
    <!-- Sub-Item Graphs: Raw Material Graphs -->
    <div id="sub-item-graphs" class="grid grid-cols-1 md:grid-cols-2 gap-8">
      {#each rawMaterials as mat (mat.ingredient)}
        <div class="bg-darker p-4 rounded-lg shadow">
          <div class="flex items-center justify-center mb-1">
            <img
              src={"https://sky.shiiyu.moe/item/" + mat.ingredient}
              alt={mat.ingredient}
              class="w-6 h-6 mr-2"
            />
            <span class="text-light font-medium">{toTitleCase(mat.ingredient)}</span>
          </div>
          {#if allAvgPrices !== null || all3DayData !== null}
            {#if findChartData(mat.ingredient) !== null}
              <AveragePriceChart
                avgData={findChartData(mat.ingredient)}
                width={320}
                height={280}
                padding={60}
              />
            {:else}
              <p class="text-sm text-light">
                No data for {toTitleCase(mat.ingredient)}
              </p>
            {/if}
          {:else}
            <p class="text-sm text-light">Loading price data...</p>
          {/if}
        </div>
      {/each}
    </div>
  </section>
  
=======
  <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
    <!-- Profit & Efficiency -->
    <div class="space-y-4">
      <h2 class="text-xl md:text-2xl font-semibold text-accent">Profit &amp; Efficiency</h2>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Craft Cost</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.crafting_cost)}
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Sell Price</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.sell_price)}
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Cycles</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.cycles_per_hour)}
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Savings</span>
        <span class="text-sm md:text-base font-medium text-light">
          ▲ {formatLargeNumber(item.crafting_savings)}
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">% Flip</span>
        <span class="text-sm md:text-base font-medium text-light bg-primary px-2 py-1 rounded">
          {formatLargeNumber(percentFlip(item))}%
        </span>
      </div>
    </div>

    <!-- Timing & Depth -->
    <div class="space-y-4">
      <h2 class="text-xl md:text-2xl font-semibold text-accent">Timing &amp; Depth</h2>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Max Depth</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.longest_step_count)}
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Buy Fill Time</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.buy_fill_time)}s
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Sell Fill Time</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.sell_fill_time)}s
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Effective Cycle Time</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.effective_cycle_time)}s
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm md:text-base text-accent">Inventory Cycles</span>
        <span class="text-sm md:text-base font-medium text-light">
          {formatLargeNumber(item.inventory_cycles)}
        </span>
      </div>
    </div>
  </div>

  <!-- 2) Recipe Tree Section -->
  <div>
    <h2 class="text-center text-2xl md:text-3xl font-bold text-light mt-0 mb-4">Recipe Tree</h2>
    <div class="relative bg-darker p-6 rounded-lg shadow-lg">
      <RecipeTree tree={item.step_breakdown} />
    </div>
  </div>

  <!-- 3) Raw Crafting Materials -->
  {#if rawMaterials.length > 0}
    <div class="mt-10">
      <h2 class="text-center text-3xl md:text-4xl font-bold text-light mb-6">
        Raw Crafting Materials
      </h2>
      <div class="max-w-md mx-auto space-y-3">
        {#each rawMaterials as material (material.ingredient)}
          <div class="grid grid-cols-[8rem_auto_1fr] items-center gap-x-6 gap-y-3">
            <!-- Right-aligned quantity -->
            <div class="text-right text-xl md:text-2xl text-accent font-semibold">
              {"x" + formatLargeNumber(material.total_needed)}
            </div>
            <!-- Icon -->
            <img
              src={"https://sky.shiiyu.moe/item/" + material.ingredient}
              alt={material.ingredient}
              class="w-8 h-8 md:w-10 md:h-10 rounded shadow-sm"
            />
            <!-- Left-aligned name -->
            <div class="text-base md:text-lg text-light font-medium">
              {toTitleCase(material.ingredient)}
            </div>
          </div>
        {/each}
      </div>
    </div>

    <!-- 4) Sub-Item Graphs -->
    <div class="mt-10">
      <h2 class="text-center text-3xl md:text-4xl font-bold text-light mb-6">
        Raw Material Price Graphs
      </h2>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
        {#each rawMaterials as mat (mat.ingredient)}
          <div class="bg-darker p-4 rounded-lg shadow">
            <!-- Label with icon -->
            <div class="flex items-center mb-2">
              <img
                src={"https://sky.shiiyu.moe/item/" + mat.ingredient}
                alt={mat.ingredient}
                class="w-6 h-6 mr-2"
              />
              <span class="text-light font-medium">{toTitleCase(mat.ingredient)}</span>
            </div>

            {#if allAvgPrices !== null || all3DayData !== null}
              <!-- Use our new function to find data in either local or 3day -->
              {#if findChartData(mat.ingredient) !== null}
                <AveragePriceChart
                  avgData={findChartData(mat.ingredient)}
                  width={320}
                  height={280}
                  padding={60}
                />
              {:else}
                <p class="text-sm text-light">No data for {toTitleCase(mat.ingredient)}</p>
              {/if}
            {:else}
              <p class="text-sm text-light">Loading price data...</p>
            {/if}
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <!-- 5) Main Item Price Graph (below raw items) -->
  <div class="mt-10">
    <h2 class="text-center text-3xl md:text-4xl font-bold text-light mb-6">
      Main Item Price History
    </h2>
    {#if (mainAvgData !== null) || (all3DayData !== null)}
      <!-- If local data is missing, try 3day fallback -->
      {#if findChartData(item.item) !== null}
        <AveragePriceChart
          avgData={findChartData(item.item)}
          width={700}
          height={300}
          padding={50}
        />
      {:else}
        <p class="text-light text-center">No average price data available for this item.</p>
      {/if}
    {:else}
      <p class="text-light text-center">No average price data available for this item.</p>
    {/if}
  </div>
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
</div>
{:else}
<div class="p-8 max-w-3xl mx-auto bg-darker rounded-lg shadow-lg">
  <p class="text-light">Item not found or data is still loading.</p>
</div>
{/if}
<<<<<<< HEAD

<style>
  /* Increase spacing between major sections */
  #stats-section,
  #recipe-tree-section,
  #item-graphs-section {
    margin-bottom: 4rem;
  }
</style>
=======
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
