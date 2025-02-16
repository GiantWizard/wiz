<script>
  import { onMount } from 'svelte';

  export let node;
  
  // Helper to convert strings like FINE_PERIDOT_GEM to "Fine Peridot Gem"
  const toTitleCase = (str) => {
    return str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (char) => char.toUpperCase());
  };

  // Helper function to check if a node is "Not craftable" or lacks further ingredients
  function isLeaf(sub) {
    return sub?.note === 'Not craftable' || !sub?.ingredients;
  }

  // (Optional) If you need any other helpers, define them here
</script>

{#if node && node.ingredients}
  <ul class="pl-4 border-l border-gray-600">
    {#each node.ingredients as ingr}
      <li class="ml-4 my-2">
        <!-- Show quantity + item name -->
        <div class="flex items-center">
          <span class="font-bold text-primary mr-2">{ingr.total_needed}x</span>
          <span class="text-light font-inter">{toTitleCase(ingr.ingredient)}</span>
        </div>

        <!-- If there's a sub_breakdown, recurse -->
        {#if ingr.sub_breakdown}
          {#if isLeaf(ingr.sub_breakdown)}
            <!-- Show leaf info, e.g. "Not craftable" -->
            {#if ingr.sub_breakdown.note === 'Not craftable'}
              <div class="text-sm text-gray-400 ml-8">
                Not craftable
              </div>
            {:else}
              <!-- If there's a final cost or other info, show it here. -->
              <div class="text-sm text-gray-400 ml-8">
                Cost: {ingr.sub_breakdown.crafting_cost}
              </div>
            {/if}
          {:else}
            <!-- Recursively render deeper breakdown -->
            <RecursiveBreakdown node={ingr.sub_breakdown} />
          {/if}
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  /* Tailwind or additional styling here if desired */
</style>
