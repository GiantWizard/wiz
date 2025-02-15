<script>
  import RecipeTree from './RecipeTree.svelte';
  export let tree;
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

<div class="node">
  <div class="node-header">{tree.item.replace(/_/g, ' ')}</div>
  {#if tree.note}
    <div class="node-note">{tree.note}</div>
  {/if}
  {#if tree.ingredients && tree.ingredients.length > 0}
    {#each tree.ingredients as ing}
      <div class="node">
        <div>
          {ing.total_needed}x {ing.ingredient.replace(/_/g, ' ')} 
          ({ing.buy_method} @ ⏣ {ing.cost_per_unit})
        </div>
        {#if ing.sub_breakdown}
          <RecipeTree tree={ing.sub_breakdown} />
        {/if}
      </div>
    {/each}
  {/if}
</div>
