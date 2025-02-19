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
  // Helper to format numbers
  const formatNumber = (
    num: number | null | undefined,
    decimals: number = 1
  ): string => {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const formatted = num.toLocaleString('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0$/, '');
  };

  // Helper to convert strings like FINE_JASPER_GEM to "Fine Jasper Gem"
  const toTitleCase = (str: string): string => {
    return str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (match: string, group1: string) => group1.toUpperCase());
  };

  // Define an interface for the item data.
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
    step_breakdown: any; // You can refine this type if needed
  }

  // Calculate percent flip
  const percentFlip = (item: BazaarItem): number => {
    if (!item.crafting_cost) return 0;
    return (item.crafting_savings / item.crafting_cost) * 100;
  };

  import RecipeTree from '$lib/components/RecipeTree.svelte';

  // Declare that the page data contains an item of type BazaarItem
  export let data: { item: BazaarItem };
  let item: BazaarItem;
  $: item = data.item;
</script>

{#if item}
  <!-- Main container -->
  <div class="p-6 max-w-4xl mx-auto bg-darker rounded-lg shadow-lg">
    <!-- Centered header info -->
    <div class="mb-6 text-center">
      <div class="text-3xl font-bold text-light font-inter mb-2">
        {toTitleCase(item.item)}
      </div>
      <div class="text-2xl font-semibold text-accent">
        {formatNumber(item.profit_per_hour)}/h
      </div>
    </div>

    <!-- Stats grid -->
    <div class="grid grid-cols-2 gap-4 mb-6">
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Craft Cost</span>
        <span class="text-sm font-medium text-light">{formatNumber(item.crafting_cost)}</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Sell Price</span>
        <span class="text-sm font-medium text-light">{formatNumber(item.sell_price)}</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Cycles</span>
        <span class="text-sm font-medium text-light">{formatNumber(item.cycles_per_hour)}</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Max Depth</span>
        <span class="text-sm font-medium text-light">{item.longest_step_count}</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Savings</span>
        <span class="text-sm font-medium text-light">▲ {formatNumber(item.crafting_savings)}</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">% Flip</span>
        <span class="text-sm font-medium text-light bg-primary px-2 py-1 rounded">
          {formatNumber(percentFlip(item))}%
        </span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Buy Fill Time</span>
        <span class="text-sm font-medium text-light">{formatNumber(item.buy_fill_time)}s</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Sell Fill Time</span>
        <span class="text-sm font-medium text-light">{formatNumber(item.sell_fill_time)}s</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Effective Cycle Time</span>
        <span class="text-sm font-medium text-light">{formatNumber(item.effective_cycle_time)}s</span>
      </div>
      <div class="flex justify-between border-b border-dark pb-1">
        <span class="text-sm text-accent">Inventory Cycles</span>
        <span class="text-sm font-medium text-light">{formatNumber(item.inventory_cycles)}</span>
      </div>
    </div>

    <!-- Recipe Tree Section -->
    <div class="text-2xl font-bold text-light mb-4">Recipe Tree</div>
    <div class="relative bg-darker-800 p-6 rounded-lg shadow-lg">
      <RecipeTree tree={item.step_breakdown} />
    </div>
  </div>
{:else}
  <div class="p-6 max-w-3xl mx-auto bg-darker rounded-lg shadow-lg">
    <p class="text-light">Item not found or data is still loading.</p>
  </div>
{/if}

<style>
  /* Additional CSS if needed; Tailwind classes handle most styling. */
</style>
