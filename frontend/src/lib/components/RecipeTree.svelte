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

  // console.log(`[RT Init D${depth}] Item: ${tree?.item}, isTopLevel: ${isTopLevel}, parentImageRef: ${!!parentImageRef}`);

  let openDropdowns: Record<string, boolean> = {};
  function toggleDropdown(id: string): void {
    openDropdowns[id] = !openDropdowns[id];
    openDropdowns = { ...openDropdowns };
  }

  function aggregateIngredients(ingredients?: TransformedIngredient[]): TransformedIngredient[] {
    if (!ingredients) return [];
    const aggregated: Record<string, TransformedIngredient> = {};
    ingredients.forEach((ing) => {
      const key = ing.ingredient;
      if (!aggregated[key]) {
        aggregated[key] = { ...ing, total_needed: 0 };
      }
      aggregated[key].total_needed += ing.total_needed;
      if (ing.sub_breakdown && !aggregated[key].sub_breakdown) aggregated[key].sub_breakdown = ing.sub_breakdown;
      if (ing.buy_method && !aggregated[key].buy_method) {
          aggregated[key].buy_method = ing.buy_method;
          aggregated[key].cost_per_unit = ing.cost_per_unit;
      }
    });
    return Object.values(aggregated);
  }

  let aggregatedIngredients: TransformedIngredient[] = [];
  $: aggregatedIngredients = aggregateIngredients(tree?.ingredients);
  $: if (aggregatedIngredients.length > 0 && typeof window !== 'undefined') {
    setTimeout(updateAllPointers, 50);
  }

  let rootElement: HTMLDivElement | null = null; 
  let containerRefs: (HTMLDivElement | null)[] = [];
  let childImageRefs: (HTMLImageElement | null)[] = []; 

  let pointerLeft: number[] = [];
  let pointerTop: number[] = [];
  let accentLeft: number[] = [];
  let accentTop: number[] = [];
  let accentWidth: number[] = [];
  let accentHeight: number[] = [];

  // --- POINTER LOGIC Constants ---
  const POINTER_DIAMETER = 34;
  const POINTER_RADIUS = POINTER_DIAMETER / 2;
  const offsetX = 14; // Adjusted: Shifts pointer circle further right
  const offsetY = -7; 
  const defaultAccentWidth = 4; 

  function updatePointer(index: number) {
    const currentIngredient = aggregatedIngredients[index];
    if (!parentImageRef || !childImageRefs[index] || !containerRefs[index] || !rootElement || !currentIngredient) {
      return;
    }

    const parentImgRect = parentImageRef.getBoundingClientRect();
    const childImgRect = childImageRefs[index]!.getBoundingClientRect();
    const ingredientItemContainerRect = containerRefs[index]!.getBoundingClientRect(); 

    if (parentImgRect.width === 0 || childImgRect.width === 0) {
      return; 
    }

    const parentCenterXRel = (parentImgRect.left + parentImgRect.width / 2) - ingredientItemContainerRect.left;
    const parentCenterYRel = (parentImgRect.top + parentImgRect.height / 2) - ingredientItemContainerRect.top;
    const childCenterYRel = (childImgRect.top + childImgRect.height / 2) - ingredientItemContainerRect.top;

    // Pointer circle's center X and Y
    pointerLeft[index] = parentCenterXRel + offsetX;
    pointerTop[index] = childCenterYRel + offsetY;

    // Accent Line Logic
    // Adjusted: Start the line further down from the parent image's center.
    const lineStartY = parentCenterYRel + (parentImgRect.height / 2) + 5; 

    // Adjusted: Shift the accent line the radius of the pointer to the left (relative to pointer circle's center)
    const accentLineCenterX = pointerLeft[index] - POINTER_RADIUS + 2; 

    accentLeft[index] = accentLineCenterX - (defaultAccentWidth / 2); 
    
    const lineEndY = pointerTop[index]; 

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
    if (parentImageRef && ((depth > 0) || (depth === 0 && isTopLevel))) {
      aggregatedIngredients.forEach((_, i) => {
        setTimeout(() => updatePointer(i), 150); 
      });
    }
  }

  onMount(() => {
    if (rootElement) { 
        if (isTopLevel && depth === 0 && parentImageRef) {
            setTimeout(updateAllPointers, 50); 
        } else if (depth > 0) {
            updateAllPointers();
        }
        window.addEventListener('resize', updateAllPointers);
        if (typeof MutationObserver !== 'undefined') {
            observer = new MutationObserver(() => {
                updateAllPointers();
            });
            observer.observe(rootElement, { childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'class'] });
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
    return { destroy() { containerRefs[index] = null; } };
  }

  function registerChildImage(node: HTMLImageElement, index: number) {
    childImageRefs[index] = node;
    const update = () => updateAllPointers();
    if (node.complete && node.naturalHeight !== 0) setTimeout(update, 0); 
    else node.onload = update;
    node.onerror = () => console.error(`[RT Image Error D${depth}] Failed: ${node.src}`);
    return { destroy() { node.onload = null; node.onerror = null; childImageRefs[index] = null; } };
  }
</script>

<style>
  li.ingredient-item { 
    position: relative; 
    min-height: 2.25rem; 
    padding-left: 1rem; 
  }
  li.ingredient-item.has-pointer {
    padding-left: 3rem; 
  }
  .pointer-container { position: relative; }
  .ingredient-pointer { 
    position: absolute; width: 34px; height: 34px; border-radius: 50%; background: #C8ACD6; 
    --b: 4px; --a: 90deg; aspect-ratio: 1; padding: var(--b);
    --_g: /var(--b) var(--b) no-repeat radial-gradient(circle at 50% 50%, #000000 97%, #0000);
    --_h: /var(--b) var(--b) no-repeat linear-gradient(90deg, #000000 100%, #0000);
    mask: top var(--_g), calc(50% + 50%*sin(var(--a))) calc(50% - 50%*cos(var(--a))) var(--_g), linear-gradient(#0000 0 0) content-box intersect, conic-gradient(#000000 var(--a), #0000 0), right 0 top 50% var(--_h);
    z-index: 20; 
    transform: translate(-50%, -50%) rotate(180deg); 
  }
  .ingredient-accent { 
    position: absolute; background-color: #C8ACD6; z-index: 10; 
  }
  .sharp-image { image-rendering: pixelated; image-rendering: crisp-edges; }
  .node-content { position: relative; z-index: 1; }
</style>

<div bind:this={rootElement}>
  {#if tree && tree.item && isTopLevel}
    <div class="mb-4">
      <div class="flex items-center gap-2 ml-[-1.5rem]"> 
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
          <li class={`ingredient-item ${parentImageRef && ((depth > 0) || (depth === 0 && isTopLevel)) ? 'has-pointer' : ''}`}>
            <div class="pointer-container" use:registerContainer={i}>
              {#if parentImageRef && ((depth > 0) || (depth === 0 && isTopLevel))}
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
                        <div class="absolute right-0 mt-1 py-1 px-2 bg-darker rounded-md shadow-lg z-30">
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