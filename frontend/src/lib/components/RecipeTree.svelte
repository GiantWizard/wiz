<script lang="ts">
  import { onMount, afterUpdate, onDestroy } from 'svelte';
  import type { TransformedTree, TransformedIngredient } from '$lib/utils/typesAndTransforms';
  import { toTitleCase, formatNumberSimple as formatNumber } from '$lib/utils/typesAndTransforms';

  // --- Props ---
  export let depth: number = 0;
  export let tree!: TransformedTree; 
  export let parentQuantity: number = 1; 
  export let isTopLevel: boolean = true;
  export let parentImageRef: HTMLImageElement | null = null;

  // --- State for UI ---
  let openDropdowns: Record<string, boolean> = {};
  function toggleDropdown(id: string): void {
    openDropdowns[id] = !openDropdowns[id];
    openDropdowns = { ...openDropdowns };
  }

  // --- Ingredient Aggregation ---
  function aggregateIngredients(ingredients?: TransformedIngredient[]): TransformedIngredient[] {
    if (!ingredients) return [];
    const aggregated: Record<string, TransformedIngredient> = {};
    ingredients.forEach((ing) => {
      const key = ing.ingredient;
      if (!aggregated[key]) {
        aggregated[key] = { ...ing, total_needed: 0 };
      }
      aggregated[key].total_needed += ing.total_needed;
      if (ing.sub_breakdown && !aggregated[key].sub_breakdown) {
          aggregated[key].sub_breakdown = ing.sub_breakdown;
      }
       if (ing.buy_method && !aggregated[key].buy_method) { 
          aggregated[key].buy_method = ing.buy_method;
          aggregated[key].cost_per_unit = ing.cost_per_unit;
      }
    });
    return Object.values(aggregated);
  }

  let aggregatedIngredients: TransformedIngredient[] = [];
  $: aggregatedIngredients = aggregateIngredients(tree?.ingredients);

  // --- POINTER LOGIC ---
  let rootElement: HTMLDivElement | null = null; 
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
  const defaultAccentWidth = 3; 
  // const pointerCircleRadius = 17; // Not directly used in style but good to know

  function updatePointer(index: number) {
    if (!parentImageRef || !childImageRefs[index] || !containerRefs[index] || !rootElement) {
      return;
    }

    // It's usually better to calculate relative to a common, non-scrolling ancestor, 
    // or ensure all getBoundingClientRect calls are in the same scroll context.
    // For simplicity here, assuming relative to viewport is okay if elements are close.
    // However, for robustness, you might pass the viewport-relative rect of `rootElement`
    // and subtract its top/left from other element rects.

    const parentImgRect = parentImageRef.getBoundingClientRect();
    const childImgRect = childImageRefs[index]!.getBoundingClientRect();
    const ingredientItemContainerRect = containerRefs[index]!.getBoundingClientRect(); 

    // Parent image center relative to the `ingredientItemContainerRect` (the `div.pointer-container`)
    const parentCenterXRel = (parentImgRect.left + parentImgRect.width / 2) - ingredientItemContainerRect.left;
    const parentCenterYRel = (parentImgRect.top + parentImgRect.height / 2) - ingredientItemContainerRect.top;
    
    // Child image center relative to the `ingredientItemContainerRect`
    const childCenterYRel = (childImgRect.top + childImgRect.height / 2) - ingredientItemContainerRect.top;

    pointerLeft[index] = parentCenterXRel + offsetX;
    pointerTop[index] = childCenterYRel + offsetY;

    const lineStartX = parentCenterXRel; 
    const lineStartY = parentCenterYRel + (parentImgRect.height / 2) * 0.5; // Start from mid-bottom of parent

    const lineEndX = pointerLeft[index]; 
    const lineEndY = pointerTop[index]; 

    // For a vertical-ish line
    accentLeft[index] = lineStartX - (defaultAccentWidth / 2); // Center the line
    accentTop[index] = Math.min(lineStartY, lineEndY);
    accentWidth[index] = defaultAccentWidth;
    accentHeight[index] = Math.abs(lineEndY - lineStartY);
    
    pointerLeft = [...pointerLeft];
    pointerTop = [...pointerTop];
    accentLeft = [...accentLeft];
    accentTop = [...accentTop];
    accentWidth = [...accentWidth];
    accentHeight = [...accentHeight];
  }
  
  let observer: MutationObserver;

  function updateAllPointers() {
    if (!aggregatedIngredients) return; 
    if (depth > 0 && parentImageRef) {
      aggregatedIngredients.forEach((_, i) => {
        setTimeout(() => updatePointer(i), 120); // Increased delay slightly more
      });
    }
  }

  onMount(() => {
    if (rootElement) { 
        updateAllPointers(); 
        window.addEventListener('resize', updateAllPointers);
        
        if (typeof MutationObserver !== 'undefined') {
            observer = new MutationObserver((mutationsList) => {
                let needsUpdate = false;
                for(const mutation of mutationsList) {
                    if (mutation.type === 'childList' || mutation.type === 'attributes') {
                        needsUpdate = true;
                        break; 
                    }
                }
                if (needsUpdate) {
                    // console.log(`[RecipeTree Observer] Mutation at depth ${depth}, updating pointers.`);
                    updateAllPointers();
                }
            });
            // Observe the root element of this component instance for changes
            observer.observe(rootElement, { childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'class'], characterData: false });
        }
    }
    return () => {
      window.removeEventListener('resize', updateAllPointers);
      if (observer) observer.disconnect();
    };
  });

  afterUpdate(() => {
     updateAllPointers();
  });

  function registerContainer(node: HTMLDivElement, index: number) {
    containerRefs[index] = node;
    return { 
      destroy() { 
        containerRefs[index] = null; 
      } 
    };
  }

  function registerChildImage(node: HTMLImageElement, index: number) {
    childImageRefs[index] = node;
    node.onload = () => {
        // console.log(`[RecipeTree Image Onload] Image loaded for depth ${depth}, index ${index}. Updating pointers.`);
        updateAllPointers(); // Re-calculate when image loads and has dimensions
    };
    // If image is already loaded (e.g. cached), onload might not fire, so also update
    if (node.complete) {
        updateAllPointers();
    }
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
    padding-left: 1rem; 
  }
  li.ingredient-item.has-pointer {
    padding-left: 2.5rem; 
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
    --b: 3px; --a: 90deg; aspect-ratio: 1; padding: var(--b);
    --_g: /var(--b) var(--b) no-repeat radial-gradient(circle at 50% 50%, #000000 97%, #0000);
    --_h: /var(--b) var(--b) no-repeat linear-gradient(90deg, #000000 100%, #0000);
    mask: top var(--_g), calc(50% + 50%*sin(var(--a))) calc(50% - 50%*cos(var(--a))) var(--_g), linear-gradient(#0000 0 0) content-box intersect, conic-gradient(#000000 var(--a), #0000 0), right 0 top 50% var(--_h);
    z-index: 2; 
    /* Centering the pointer circle on its left/top coordinates */
    transform: translate(-50%, -50%) rotate(180deg); 
  }
  .ingredient-accent { 
    position: absolute; 
    background-color: #C8ACD6; 
    z-index: 1; 
  }
  .sharp-image { 
    image-rendering: -moz-crisp-edges; 
    image-rendering: -webkit-crisp-edges; 
    image-rendering: pixelated; 
    image-rendering: crisp-edges; 
  }
  .node-content {
    position: relative; /* To ensure it's in the flow and z-index works as expected relative to pointers */
    z-index: 0; /* Below pointers and accents if they overlap */
  }
</style>

<div bind:this={rootElement}>
  {#if tree && tree.item && isTopLevel}
    <div class="mb-4">
      <!-- Corrected comment placement -->
      <div class="flex items-center gap-2 ml-[-1.5rem]"> <!-- Negative margin for alignment -->
        <img
          bind:this={parentImageRef} 
          src={`https://sky.coflnet.com/static/icon/${tree.item}`}
          alt={tree.item || 'item'}
          class="w-8 h-8 rounded-sm shadow-sm sharp-image"
        />
        <span class="text-2xl font-semibold text-light">
          {toTitleCase(tree.item)}
        </span>
      </div>
    </div>
  {/if}

  {#if aggregatedIngredients && aggregatedIngredients.length > 0}
    <div style={depth === 0 && isTopLevel ? "transform: translateX(-20px) translateY(-5px);" : ""}>
      <ul class="space-y-6">
        {#each aggregatedIngredients as ing, i (ing.ingredient + i + depth)}
          <li class={`ingredient-item ${depth > 0 && parentImageRef ? 'has-pointer' : ''}`}>
            <div class="pointer-container" use:registerContainer={i}>
              {#if parentImageRef && depth > 0}
                <div 
                  class="ingredient-accent" 
                  style="left: {accentLeft[i] || 0}px; top: {accentTop[i] || 0}px; width: {accentWidth[i] || 0}px; height: {accentHeight[i] || 0}px;"
                ></div>
                <div 
                  class="ingredient-pointer" 
                  style="left: {pointerLeft[i] || 0}px; top: {pointerTop[i] || 0}px;" 
                  aria-hidden="true"
                ></div>
              {/if}
              <div class="node-content"> 
                <div class="flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <img
                      use:registerChildImage={i} 
                      src={`https://sky.coflnet.com/static/icon/${ing.ingredient}`}
                      alt={ing.ingredient}
                      class="w-8 h-8 rounded-sm shadow-sm sharp-image"
                    />
                    <span class="text-xl font-bold text-bright">
                      {formatNumber(ing.total_needed * parentQuantity, (ing.total_needed * parentQuantity) % 1 === 0 ? 0 : 1)}x
                    </span>
                    <span class="text-xl text-light">
                      {toTitleCase(ing.ingredient)}
                    </span>
                  </div>
                  {#if ing.buy_method && ing.cost_per_unit != null}
                    <div class="relative">
                      <button
                        class="flex items-center gap-1 text-xl text-gray-400 hover:text-accent focus:outline-none"
                        on:click={() => toggleDropdown(ing.ingredient + i)}
                      >
                        <span>{formatNumber(ing.cost_per_unit, ing.cost_per_unit < 1 && ing.cost_per_unit !== 0 ? 2 : 0)} Each</span>
                        <svg class="w-4 h-4 transform transition-transform {openDropdowns[ing.ingredient + i] ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
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

            {#if ing.sub_breakdown}
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
    </div>
  {/if}
</div>