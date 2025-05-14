<script lang="ts">
  import { onMount, afterUpdate, onDestroy } from 'svelte';

  // --- Type Definitions ---
  interface Tree {
    item?: string;
    note?: any;
    ingredients?: Ingredient[];
  }

  interface Ingredient {
    ingredient: string;
    total_needed: number;
    buy_method?: string;
    cost_per_unit?: number;
    sub_breakdown?: Tree;
  }

  // --- Props ---
  export let depth: number = 0;
  export let tree!: Tree;
  export let parentQuantity: number = 1;
  export let isTopLevel: boolean = true;
  // Parent's image reference (if provided)
  export let parentImageRef: HTMLImageElement | null = null;

  // Toggling sub-breakdown cost info
  let openDropdowns: Record<string, boolean> = {};
  function toggleDropdown(id: string): void {
    openDropdowns[id] = !openDropdowns[id];
    // Force reactivity
    openDropdowns = { ...openDropdowns };
  }

  // Utility functions
  function toTitleCase(str: string): string {
    return str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (_match, char) => char.toUpperCase());
  }

  function formatNumber(num: number, decimals: number = 1): string {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const formatted = num.toLocaleString('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0$/, '');
  }

  function aggregateIngredients(ingredients: Ingredient[]): Ingredient[] {
    const aggregated: Record<string, Ingredient> = {};
    ingredients.forEach((ing) => {
      const key = ing.ingredient;
      if (!aggregated[key]) {
        aggregated[key] = { ...ing, total_needed: 0 };
      }
      aggregated[key].total_needed += ing.total_needed;
    });
    return Object.values(aggregated);
  }

  // Reactive aggregated ingredients
  let aggregatedIngredients: Ingredient[] = [];
  $: aggregatedIngredients = tree.ingredients
    ? aggregateIngredients(tree.ingredients)
    : [];

  // --- POINTER LOGIC (inlined) ---

  // Arrays to store per-ingredient DOM refs and computed values
  let containerRefs: (HTMLDivElement | null)[] = [];
  let childImageRefs: (HTMLImageElement | null)[] = [];
  let pointerLeft: number[] = [];
  let pointerTop: number[] = [];
  let accentLeft: number[] = [];
  let accentTop: number[] = [];
  let accentWidth: number[] = [];
  let accentHeight: number[] = [];

  const offsetX = 15;
  const offsetY = -15;
  const defaultAccentWidth = 34 / 10.5;

  // Computes pointer positions for the ingredient at index i
  function updatePointer(i: number) {
    if (!parentImageRef || !childImageRefs[i] || !containerRefs[i]) return;

    const containerRect = containerRefs[i]!.getBoundingClientRect();
    const parentRect = parentImageRef.getBoundingClientRect();
    const childRect = childImageRefs[i]!.getBoundingClientRect();

    const parentCenterX =
      (parentRect.left + parentRect.width / 2) - containerRect.left;
    const parentCenterY =
      (parentRect.top + parentRect.height / 2) - containerRect.top;
    const childCenterY =
      (childRect.top + childRect.height / 2) - containerRect.top;

    pointerLeft[i] = parentCenterX + offsetX;
    pointerTop[i] = childCenterY + offsetY;

    accentLeft[i] = parentCenterX - 2;
    const lineStart = parentCenterY + 18;
    const lineEnd = pointerTop[i];
    if (lineEnd >= lineStart) {
      accentTop[i] = lineStart;
      accentHeight[i] = lineEnd - lineStart;
    } else {
      accentTop[i] = lineEnd;
      accentHeight[i] = lineStart - lineEnd;
    }
    accentWidth[i] = defaultAccentWidth;
  }

  // Update pointers for all ingredients
  function updateAllPointers() {
    aggregatedIngredients.forEach((_, i) => {
      // small delay to let the DOM settle
      setTimeout(() => updatePointer(i), 10);
    });
  }

  onMount(() => {
    updateAllPointers();
    window.addEventListener('resize', updateAllPointers);
    return () => {
      window.removeEventListener('resize', updateAllPointers);
    };
  });

  afterUpdate(() => {
    updateAllPointers();
  });

  // --- Custom actions to register DOM references ---

  // Registers container div for ingredient at index "index"
  function registerContainer(node: HTMLDivElement, index: number) {
    containerRefs[index] = node;
    return {
      destroy() {
        containerRefs[index] = null;
      }
    };
  }

  // Registers child image for ingredient at index "index"
  function registerChildImage(node: HTMLImageElement, index: number) {
    childImageRefs[index] = node;
    return {
      destroy() {
        childImageRefs[index] = null;
      }
    };
  }
</script>

<style>
  li.ingredient-item {
    position: relative;
    min-height: 2.25rem;
  }

  .pointer-container {
    position: relative;
  }

  .ingredient-pointer {
    position: absolute;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: #C8ACD6;
    /* swirl pattern example */
    --b: 3px;
    --a: 90deg;
    aspect-ratio: 1;
    padding: var(--b);
    --_g: /var(--b) var(--b) no-repeat radial-gradient(circle at 50% 50%, #000 97%, #0000);
    --_h: /var(--b) var(--b) no-repeat linear-gradient(90deg, #000 100%, #0000);
    mask: top var(--_g),
          calc(50% + 50%*sin(var(--a)))
          calc(50% - 50%*cos(var(--a))) var(--_g),
          linear-gradient(#0000 0 0) content-box intersect,
          conic-gradient(#000 var(--a), #0000 0),
          right 0 top 50% var(--_h);
    z-index: 2;
    transform: rotate(180deg);
  }

  .ingredient-accent {
    position: absolute;
    background-color: #C8ACD6;
    z-index: 1;
  }
</style>

<div>
  <!-- Top-level header -->
  {#if tree.item && !tree.note && isTopLevel}
    <div class="mb-4">
      <div class="flex items-center gap-2 ml-[-1.5rem]">
        <img
          bind:this={parentImageRef}
          src={"https://sky.shiiyu.moe/item/" + tree.item}
          alt={tree.item || 'item'}
          class="w-8 h-8 rounded-sm shadow-sm"
        />
        <span class="text-2xl font-semibold text-light">
          {toTitleCase(tree.item)}
        </span>
      </div>
    </div>
  {/if}

  <!-- Render aggregated ingredients -->
  {#if aggregatedIngredients.length > 0}
    {#if depth === 0}
      <div style="transform: translateX(-20px) translateY(-5px);">
        <ul class="space-y-6">
          {#each aggregatedIngredients as ing, i}
            <li class="pl-8 ingredient-item">
              <div class="pointer-container" use:registerContainer={i}>
                <!-- Accent line -->
                <div
                  class="ingredient-accent"
                  style="left: {accentLeft[i]}px; top: {accentTop[i]}px; width: {accentWidth[i]}px; height: {accentHeight[i]}px;"
                ></div>
                <!-- Pointer -->
                <div
                  class="ingredient-pointer"
                  style="left: {pointerLeft[i]}px; top: {pointerTop[i]}px; transform: translate(-50%, -50%) rotate(180deg);"
                ></div>

                <div class="node-content">
                  <div class="flex items-center justify-between gap-2">
                    <div class="flex items-center gap-2">
                      <img
                        use:registerChildImage={i}
                        src={"https://sky.shiiyu.moe/item/" + ing.ingredient}
                        alt={ing.ingredient}
                        class="w-8 h-8 rounded-sm shadow-sm"
                      />
                      <span class="text-xl font-bold text-bright">
                        {formatNumber(ing.total_needed * parentQuantity)}x
                      </span>
                      <span class="text-xl text-light">
                        {toTitleCase(ing.ingredient)}
                      </span>
                    </div>
                    {#if ing.buy_method}
                      <div class="relative">
                        <button
                          class="flex items-center gap-1 text-xl text-gray-400 hover:text-accent focus:outline-none"
                          on:click={() => toggleDropdown(ing.ingredient + i)}
                        >
                          <span>{formatNumber(ing.cost_per_unit || 0)} Each</span>
                          <svg
                            class="w-4 h-4 transform transition-transform {openDropdowns[ing.ingredient + i] ? 'rotate-180' : ''}"
                            fill="none"
                            stroke="currentColor"
                            viewBox="0 0 24 24"
                          >
                            <path
                              stroke-linecap="round"
                              stroke-linejoin="round"
                              stroke-width="2"
                              d="M19 9l-7 7-7-7"
                            />
                          </svg>
                        </button>
                        {#if openDropdowns[ing.ingredient + i]}
                          <div class="absolute right-0 mt-1 py-1 px-2 bg-darker rounded-md shadow-lg z-10">
                            <span class="text-sm text-gray-400 whitespace-nowrap">
                              {ing.buy_method}
                            </span>
                          </div>
                        {/if}
                      </div>
                    {/if}
                  </div>
                </div>
              </div>

              {#if ing.sub_breakdown && !ing.sub_breakdown.note}
                <div class="mt-2">
                  <!-- Recursive render; pass this ingredient’s image as the parent's ref -->
                  <svelte:self
                    tree={ing.sub_breakdown}
                    parentQuantity={ing.total_needed * parentQuantity}
                    isTopLevel={false}
                    parentImageRef={childImageRefs[i]}
                    depth={depth + 1}
                  />
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {:else}
      <!-- Deeper levels (no transform) -->
      <ul class="space-y-6">
        {#each aggregatedIngredients as ing, i}
          <li class="pl-8 ingredient-item">
            <div class="pointer-container" use:registerContainer={i}>
              <!-- Accent line -->
              <div
                class="ingredient-accent"
                style="left: {accentLeft[i]}px; top: {accentTop[i]}px; width: {accentWidth[i]}px; height: {accentHeight[i]}px;"
              ></div>
              <!-- Pointer -->
              <div
                class="ingredient-pointer"
                style="left: {pointerLeft[i]}px; top: {pointerTop[i]}px; transform: translate(-50%, -50%) rotate(180deg);"
              ></div>

              <div class="node-content">
                <div class="flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <img
                      use:registerChildImage={i}
                      src={"https://sky.shiiyu.moe/item/" + ing.ingredient}
                      alt={ing.ingredient}
                      class="w-8 h-8 rounded-sm shadow-sm"
                    />
                    <span class="text-xl font-bold text-bright">
                      {formatNumber(ing.total_needed * parentQuantity)}x
                    </span>
                    <span class="text-xl text-light">
                      {toTitleCase(ing.ingredient)}
                    </span>
                  </div>
                  {#if ing.buy_method}
                    <div class="relative">
                      <button
                        class="flex items-center gap-1 text-xl text-gray-400 hover:text-accent focus:outline-none"
                        on:click={() => toggleDropdown(ing.ingredient + i)}
                      >
                        <span>{formatNumber(ing.cost_per_unit || 0)} Each</span>
                        <svg
                          class="w-4 h-4 transform transition-transform {openDropdowns[ing.ingredient + i] ? 'rotate-180' : ''}"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M19 9l-7 7-7-7"
                          />
                        </svg>
                      </button>
                      {#if openDropdowns[ing.ingredient + i]}
                        <div class="absolute right-0 mt-1 py-1 px-2 bg-darker rounded-md shadow-lg z-10">
                          <span class="text-sm text-gray-400 whitespace-nowrap">
                            {ing.buy_method}
                          </span>
                        </div>
                      {/if}
                    </div>
                  {/if}
                </div>
              </div>
            </div>

            {#if ing.sub_breakdown && !ing.sub_breakdown.note}
              <div class="mt-2">
                <svelte:self
                  tree={ing.sub_breakdown}
                  parentQuantity={ing.total_needed * parentQuantity}
                  isTopLevel={false}
                  parentImageRef={childImageRefs[i]}
                  depth={depth + 1}
                />
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>
