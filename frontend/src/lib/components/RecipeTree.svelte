<script>
  export let tree;
  export let parentQuantity = 1;
  export let isTopLevel = true;

  let openDropdowns = {};

  function toggleDropdown(id) {
    openDropdowns[id] = !openDropdowns[id];
    openDropdowns = openDropdowns; // trigger reactivity
  }

  // Helper to convert strings like FINE_PERIDOT_GEM to "Fine Peridot Gem"
  function toTitleCase(str) {
    return str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (char) => char.toUpperCase());
  }

  // Helper to format numbers
  function formatNumber(num, decimals = 1) {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const formatted = num.toLocaleString('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0$/, '');
  }

  // Helper to aggregate ingredients at the same level
  function aggregateIngredients(ingredients) {
    const aggregated = {};
    
    ingredients.forEach(ing => {
      const key = ing.ingredient;
      if (!aggregated[key]) {
        aggregated[key] = {
          ...ing,
          total_needed: 0
        };
      }
      aggregated[key].total_needed += ing.total_needed;
    });

    return Object.values(aggregated);
  }
</script>

<style>
  .node {
    margin-left: 1rem;
    border-left: 1px dashed #ccc;
    padding-left: 0.5rem;
    margin-bottom: 0.5rem;
  }
  .node-header {
    font-weight: bold;
  }
  .node-note {
    font-style: italic;
    color: #6B7280;
  }
</style>

<div class="pl-2">
  <!-- Item header -->
  {#if tree.item && !tree.note && isTopLevel}
    <div class="mb-4">
      <div class="flex items-center gap-2">
        <img
          src={"https://sky.shiiyu.moe/item/" + tree.item}
          alt={tree.item}
          class="w-8 h-8 rounded-sm shadow-sm relative -left-3.5"
        />
        <span class="text-2xl font-semibold text-light">{toTitleCase(tree.item)}</span>
      </div>
    </div>
  {/if}

  <!-- Ingredients list -->
  {#if tree.ingredients && tree.ingredients.length > 0}
    <ul class="space-y-6">
      {#each aggregateIngredients(tree.ingredients) as ing, i}
        <li class="pl-4 border-l-2 border-accent" style="min-height: 2.25rem">
          <div class="node-content">
            <div class="flex items-center justify-between gap-2">
              <div class="flex items-center gap-2">
                <span class="text-xl font-bold text-bright">{formatNumber(ing.total_needed * parentQuantity)}x</span>
                <img
                  src={"https://sky.shiiyu.moe/item/" + ing.ingredient}
                  alt={ing.ingredient}
                  class="w-8 h-8 rounded-sm shadow-sm"
                />
                <span class="text-xl text-light">{toTitleCase(ing.ingredient)}</span>
              </div>
              {#if ing.buy_method}
                <div class="relative">
                  <button 
                    class="flex items-center gap-1 text-xl text-gray-400 hover:text-accent focus:outline-none"
                    on:click={() => toggleDropdown(ing.ingredient + i)}
                  >
                    <span>{formatNumber(ing.cost_per_unit)} Each</span>
                    <svg 
                      class="w-4 h-4 transform transition-transform {openDropdowns[ing.ingredient + i] ? 'rotate-180' : ''}" 
                      fill="none" 
                      stroke="currentColor" 
                      viewBox="0 0 24 24"
                    >
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                    </svg>
                  </button>
                  {#if openDropdowns[ing.ingredient + i]}
                    <div 
                      class="absolute right-0 mt-1 py-1 px-2 bg-darker rounded-md shadow-lg z-10"
                    >
                      <span class="text-sm text-gray-400 whitespace-nowrap">
                        {ing.buy_method}
                      </span>
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          </div>

          <!-- Recursively render sub-breakdowns -->
          {#if ing.sub_breakdown && !ing.sub_breakdown.note}
            <div class="mt-2">
              <svelte:self 
                tree={ing.sub_breakdown} 
                parentQuantity={ing.total_needed * parentQuantity}
                isTopLevel={false}
              />
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
