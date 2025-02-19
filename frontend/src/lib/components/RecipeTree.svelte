<script>
  import { onMount } from 'svelte';

  // Props
  export let depth = 0;
  export let tree;
  export let parentQuantity = 1;
  export let isTopLevel = true;
  export let parentImageRef; // parent's image reference

  // Toggling sub-breakdown cost info
  let openDropdowns = {};

  function toggleDropdown(id) {
    openDropdowns[id] = !openDropdowns[id];
    // Force reactivity
    openDropdowns = openDropdowns;
  }

  // Converts "FINE_PERIDOT_GEM" -> "Fine Peridot Gem"
  function toTitleCase(str) {
    return str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (char) => char.toUpperCase());
  }

  // Formats numbers with default 1 decimal place
  function formatNumber(num, decimals = 1) {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const formatted = num.toLocaleString('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0$/, '');
  }

  // Aggregates identical ingredients at the same level
  function aggregateIngredients(ingredients) {
    const aggregated = {};
    ingredients.forEach((ing) => {
      const key = ing.ingredient;
      if (!aggregated[key]) {
        aggregated[key] = { ...ing, total_needed: 0 };
      }
      aggregated[key].total_needed += ing.total_needed;
    });
    return Object.values(aggregated);
  }

  // -----------------------------
  // DYNAMIC POINTER & ACCENT
  // -----------------------------
  let containerRef;
  let childImageRef;
  let pointerWidth = 34;

  // Pointer coordinates
  let pointerLeft = 0;
  let pointerTop = 0;
  let offsetX = 15;
  let offsetY = -15;

  // Accent line
  let accentLeft = 0;
  let accentTop = 0;
  let accentWidth = pointerWidth;
  let accentHeight = 0;

  function updatePointer() {
    if (parentImageRef && childImageRef && containerRef) {
      const containerRect = containerRef.getBoundingClientRect();
      const parentRect = parentImageRef.getBoundingClientRect();
      const childRect = childImageRef.getBoundingClientRect();

      // Parent center in container coords
      const parentCenterX = (parentRect.left + parentRect.width / 2) - containerRect.left;
      const parentCenterY = (parentRect.top + parentRect.height / 2) - containerRect.top;

      // Pointer center (aligned with child's center + offset)
      const childCenterY = (childRect.top + childRect.height / 2) - containerRect.top;
      pointerLeft = parentCenterX + offsetX;
      pointerTop = childCenterY + offsetY;

      // Accent line: from (parentCenterY - 8) down to pointerTop
      accentLeft = parentCenterX-2;
      accentWidth = pointerWidth / 10.5;

      // Decide which is "top" and which is "bottom" so height is always positive
      const lineStart = parentCenterY + 18;
      const lineEnd = pointerTop;

      if (lineEnd >= lineStart) {
        accentTop = lineStart;
        accentHeight = lineEnd - lineStart;
      } else {
        accentTop = lineEnd;
        accentHeight = lineStart - lineEnd;
      }
    }
  }

  onMount(() => {
    updatePointer();
    window.addEventListener('resize', updatePointer);
    return () => window.removeEventListener('resize', updatePointer);
  });
</script>

<style>
  /* Pointer styling */
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
    transform: rotate(180deg);
    z-index: 2; /* Ensure pointer is on top */
  }

  /* Solid accent line behind pointer, same color */
  .ingredient-accent {
    position: absolute;
    background-color: #C8ACD6;
    z-index: 1; /* behind pointer */
  }

  li.ingredient-item {
    position: relative;
    min-height: 2.25rem;
  }
</style>

<div>
  <!-- If top-level item, show header -->
  {#if tree.item && !tree.note && isTopLevel}
    <div class="mb-4">
      <div class="flex items-center gap-2 ml-[-1.5rem]">
        <img
          bind:this={parentImageRef}
          src={"https://sky.shiiyu.moe/item/" + tree.item}
          alt={tree.item}
          class="w-8 h-8 rounded-sm shadow-sm"
        />
        <span class="text-2xl font-semibold text-light">
          {toTitleCase(tree.item)}
        </span>
      </div>
    </div>
  {/if}

  <!-- List ingredients if present -->
  {#if tree.ingredients && tree.ingredients.length > 0}
    {#if depth === 0}
      <!-- Shift top-level group left by 20px -->
      <div style="transform: translateX(-20px) translateY(-5px);">
        <ul class="space-y-6">
          {#each aggregateIngredients(tree.ingredients) as ing, i}
            <li class="pl-8 ingredient-item">
              <div bind:this={containerRef} style="position: relative;">
                <!-- Accent line -->
                <div
                  class="ingredient-accent"
                  style="
                    left: {accentLeft}px;
                    top: {accentTop}px;
                    width: {accentWidth}px;
                    height: {accentHeight}px;
                  "
                ></div>

                <!-- Pointer -->
                <div
                  class="ingredient-pointer text-accent"
                  style="
                    left: {pointerLeft}px;
                    top: {pointerTop}px;
                    transform: translate(-50%, -50%) rotate(180deg);
                  "
                ></div>

                <div class="node-content">
                  <div class="flex items-center justify-between gap-2">
                    <div class="flex items-center gap-2">
                      <img
                        bind:this={childImageRef}
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
                          <span>{formatNumber(ing.cost_per_unit)} Each</span>
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
              </div>

              {#if ing.sub_breakdown && !ing.sub_breakdown.note}
                <div class="mt-2">
                  <svelte:self
                    tree={ing.sub_breakdown}
                    parentQuantity={ing.total_needed * parentQuantity}
                    isTopLevel={false}
                    parentImageRef={childImageRef}
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
        {#each aggregateIngredients(tree.ingredients) as ing, i}
          <li class="pl-8 ingredient-item">
            <div bind:this={containerRef} style="position: relative;">
              <!-- Accent line -->
              <div
                class="ingredient-accent"
                style="
                  left: {accentLeft}px;
                  top: {accentTop}px;
                  width: {accentWidth}px;
                  height: {accentHeight}px;
                "
              ></div>

              <!-- Pointer -->
              <div
                class="ingredient-pointer text-accent"
                style="
                  left: {pointerLeft}px;
                  top: {pointerTop}px;
                  transform: translate(-50%, -50%) rotate(180deg);
                "
              ></div>

              <div class="node-content">
                <div class="flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <img
                      bind:this={childImageRef}
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
                        <span>{formatNumber(ing.cost_per_unit)} Each</span>
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
            </div>

            {#if ing.sub_breakdown && !ing.sub_breakdown.note}
              <div class="mt-2">
                <svelte:self
                  tree={ing.sub_breakdown}
                  parentQuantity={ing.total_needed * parentQuantity}
                  isTopLevel={false}
                  parentImageRef={childImageRef}
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
