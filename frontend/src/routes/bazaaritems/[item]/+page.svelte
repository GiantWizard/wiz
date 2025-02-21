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
  // Helpers
  const formatNumber = (num: number | null | undefined, decimals: number = 1): string => {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const formatted = num.toLocaleString('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0$/, '');
  };

  const toTitleCase = (str: string): string =>
    str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (_, g1) => g1.toUpperCase());

  // Types
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

  // Percent flip helper
  const percentFlip = (item: BazaarItem): number => {
    if (!item.crafting_cost) return 0;
    return (item.crafting_savings / item.crafting_cost) * 100;
  };

  import RecipeTree from '$lib/components/RecipeTree.svelte';

  // Declare page data
  export let data: { item: BazaarItem };
  let item: BazaarItem;
  $: item = data.item;

  // Recursive function to aggregate raw materials (non-craftable ingredients).
  function getRawMaterials(tree: Tree, multiplier: number = 1): Ingredient[] {
    const rawMap: Record<string, Ingredient> = {};
    if (tree.ingredients) {
      tree.ingredients.forEach(ing => {
        const total = ing.total_needed * multiplier;
        // If it has a further breakdown (no note), recurse deeper
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
</script>

{#if item}
  <!-- Main container with increased spacing between sections -->
  <div class="p-8 max-w-4xl mx-auto bg-darker rounded-lg shadow-lg space-y-12">
    <!-- Header -->
    <div class="text-center space-y-2">
      <div class="text-3xl md:text-4xl font-bold text-light font-inter">
        {toTitleCase(item.item)}
      </div>
      <div class="text-2xl md:text-3xl font-semibold text-accent">
        {formatNumber(item.profit_per_hour)}/h
      </div>
    </div>

    <!-- Stats Section -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
      <!-- Profit & Efficiency -->
      <div class="space-y-4">
        <h2 class="text-xl md:text-2xl font-semibold text-accent">Profit &amp; Efficiency</h2>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Craft Cost</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatNumber(item.crafting_cost)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Sell Price</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatNumber(item.sell_price)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Cycles</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatNumber(item.cycles_per_hour)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Savings</span>
          <span class="text-sm md:text-base font-medium text-light">
            ▲ {formatNumber(item.crafting_savings)}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">% Flip</span>
          <span class="text-sm md:text-base font-medium text-light bg-primary px-2 py-1 rounded">
            {formatNumber(percentFlip(item))}%
          </span>
        </div>
      </div>

      <!-- Timing & Depth -->
      <div class="space-y-4">
        <h2 class="text-xl md:text-2xl font-semibold text-accent">Timing &amp; Depth</h2>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Max Depth</span>
          <span class="text-sm md:text-base font-medium text-light">
            {item.longest_step_count}
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Buy Fill Time</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatNumber(item.buy_fill_time)}s
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Sell Fill Time</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatNumber(item.sell_fill_time)}s
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Effective Cycle Time</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatNumber(item.effective_cycle_time)}s
          </span>
        </div>
        <div class="flex justify-between border-b border-dark pb-1">
          <span class="text-sm md:text-base text-accent">Inventory Cycles</span>
          <span class="text-sm md:text-base font-medium text-light">
            {formatNumber(item.inventory_cycles)}
          </span>
        </div>
      </div>
    </div>

    <!-- Recipe Tree Section -->
    <div>
      <h2 class="text-center text-2xl md:text-3xl font-bold text-light mt-0 mb-4">Recipe Tree</h2>
      <div class="relative bg-darker p-6 rounded-lg shadow-lg">
        <RecipeTree tree={item.step_breakdown} />
      </div>
    </div>

    <!-- Raw Crafting Materials Section -->
    {#if rawMaterials.length > 0}
    <div class="mt-10">
      <h2 class="text-center text-2xl md:text-3xl font-bold text-light mt-0 mb-4">
        Raw Crafting Materials
      </h2>
      <div class="max-w-md mx-auto space-y-3">
        {#each rawMaterials as material (material.ingredient)}
          <div class="grid grid-cols-[6rem_auto_1fr] items-center gap-x-4">
            <div class="text-right text-xl md:text-2xl text-accent font-semibold">
              {"x" + formatNumber(material.total_needed)}
            </div>
            <img
              src={"https://sky.shiiyu.moe/item/" + material.ingredient}
              alt={material.ingredient}
              class="w-8 h-8 md:w-10 md:h-10 rounded shadow-sm"
            />
            <div class="text-base md:text-lg text-light font-medium">
              {toTitleCase(material.ingredient)}
            </div>
          </div>
        {/each}
      </div>
    </div>
    {/if}

  </div>
{:else}
  <div class="p-8 max-w-3xl mx-auto bg-darker rounded-lg shadow-lg">
    <p class="text-light">Item not found or data is still loading.</p>
  </div>
{/if}

<style>
  /* You can add small overrides or additional styling here if needed. */
</style>
