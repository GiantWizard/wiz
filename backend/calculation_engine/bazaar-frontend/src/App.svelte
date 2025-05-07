<!-- src/App.svelte -->
<script>
  import { onMount } from 'svelte';

  let item = 'PRECURSOR_APPARATUS'; // Default item
  let qty = 1;          // Default quantity
  let dualResult = null;
  let error = null;
  let loading = false;

  async function calculate() { /* ... No changes ... */
    error = null; dualResult = null; loading = true;
    const normalizedItem = item.toUpperCase().trim();
    if (!normalizedItem) { error = "Item ID cannot be empty."; loading = false; return; }
    const params = new URLSearchParams({ item: normalizedItem, qty: String(qty) });
    try {
      console.log(`Fetching: /api/expand-dual?${params}`);
      const res = await fetch(`/api/expand-dual?${params}`);
      if (!res.ok) { const errorText = await res.text(); console.error(`API Error ${res.status}: ${errorText}`); throw new Error(`API Error ${res.status}: ${errorText || res.statusText}`); }
      dualResult = await res.json();
      console.log("API Dual Result:", dualResult);
    } catch (e) { console.error("Fetch/Calculation Error:", e); error = e.message || 'An unknown error occurred.';
    } finally { loading = false; }
   }

  onMount(calculate);

  function handleClick() { if (!loading) { calculate(); } }

  function formatNum(num, digits = 2) { /* ... No changes ... */
      if (num === null || num === undefined) return 'N/A';
      if (isNaN(num)) return 'N/A';
      if (!isFinite(num)) return num > 0 ? 'Infinite' : '-Infinite';
      return Number(num).toFixed(digits);
  }

  function formatTime(num) { /* ... No changes ... */
       if (num === null || num === undefined || isNaN(num)) return 'N/A';
       if (!isFinite(num)) return num > 0 ? 'Infinite' : 'N/A (-Inf)';
       if (num < 0) return 'N/A (<0)'; if (num === 0) return '0.0s';
       if (num < 1) return num.toFixed(2) + 's'; if (num < 60) return num.toFixed(1) + 's';
       let mins = num / 60; if (mins < 60) return mins.toFixed(1) + 'm';
       let hours = mins / 60; if (hours < 24) return hours.toFixed(1) + 'h';
       let days = hours / 24; return days.toFixed(1) + 'd';
  }

  function getBaseIngredientsArray(perspectiveResult) { /* ... No changes ... */
      const ingredientsMap = perspectiveResult?.base_ingredients ?? {};
      const ingredientsArray = Object.entries(ingredientsMap).map(([name, detail]) => ({
          name, qty: detail.quantity, assocCost: detail.associated_cost, bestCost: detail.best_cost,
          pricePerUnit: detail.quantity > 0 ? detail.associated_cost / detail.quantity : NaN,
          method: detail.method, rr: detail.rr
      }));
      return ingredientsArray.sort((a, b) => a.name.localeCompare(b.name));
  }

  function getPerspectiveSummary(perspective, topLevelQty) { /* ... No changes ... */
      if (!perspective || !perspective.calculation_possible || topLevelQty <= 0) { return { pricePerUnitCrafted: NaN, profitPerUnit: NaN, totalRevenue: NaN, totalProfit: NaN, topSellPrice: NaN }; }
      const pricePerUnitCrafted = perspective.total_cost / topLevelQty;
      const estTopSellPrice = perspective.top_level_cost / topLevelQty;
      const totalRevenue = estTopSellPrice * topLevelQty;
      const profitPerUnit = estTopSellPrice - pricePerUnitCrafted;
      const totalProfit = totalRevenue - perspective.total_cost;
      return { pricePerUnitCrafted, profitPerUnit, totalRevenue, totalProfit, topSellPrice: estTopSellPrice };
  }

</script>

<main>
  <h1>Bazaar Dual Expansion Dashboard</h1>

  <div class="controls"> <!-- ... Controls remain the same ... -->
     <label>Item ID: <input bind:value={item} placeholder="e.g. DIAMOND" disabled={loading} on:keyup={(e) => e.key === 'Enter' && handleClick()} /></label>
     <label>Quantity: <input type="number" min="1" step="1" bind:value={qty} disabled={loading} on:keyup={(e) => e.key === 'Enter' && handleClick()} /></label>
     <button on:click={handleClick} disabled={loading}> {#if loading}Calculating...{:else}Calculate{/if} </button>
  </div>

  {#if error} <p class="error">Error: {error}</p> {/if}

  {#if dualResult && !error}
    {@const primary = dualResult.primary_based}
    {@const secondary = dualResult.secondary_based}
    {@const primaryIngredients = getBaseIngredientsArray(primary)}
    {@const secondaryIngredients = getBaseIngredientsArray(secondary)}
    {@const primarySummary = getPerspectiveSummary(primary, dualResult.quantity)}
    {@const secondarySummary = getPerspectiveSummary(secondary, dualResult.quantity)}

      <!-- Summary Grid -->
      <div class="results-grid">
         <!-- Perspective 1 Summary -->
         <div class="perspective">
             <h2>Perspective: Primary C10M Based</h2>
             {#if primary.calculation_possible} <p class="status-ok">Calculation OK</p> {:else} <p class="status-error">Calculation Failed</p> {/if}
             <p><strong>Top-Level Action:</strong> {primary.top_level_action ?? 'N/A'}</p>
             {#if !primary.calculation_possible && primary.error_message}<p class="reason"><strong>Reason:</strong> {primary.error_message}</p>{/if}
             <p><strong>Final Cost Method:</strong> {primary.final_cost_method ?? 'N/A'} </p>
             <p><strong>Total Estimated Cost:</strong> <span class="cost">{formatNum(primary.total_cost, 2)}</span></p>
             {#if primary.calculation_possible}
                <p><strong>Price Per Unit (Crafted):</strong> {formatNum(primarySummary.pricePerUnitCrafted, 2)}</p>
                <p><strong>Est. Profit Per Unit:</strong> {formatNum(primarySummary.profitPerUnit, 2)} <i>(vs benchmark)</i></p>
             {/if}
             <p><i>(Benchmark Primary Cost: {formatNum(primary.top_level_cost, 2)})</i></p>
             {#if primary.top_level_rr !== null && primary.top_level_rr !== undefined } <p><i>(Top-Level Primary RR: {formatNum(primary.top_level_rr, 2)})</i></p> {/if}
         </div>
         <!-- Perspective 2 Summary -->
         <div class="perspective">
              <h2>Perspective: Secondary Based</h2>
              {#if secondary.calculation_possible} {#if secondary.error_message && secondary.top_level_action !== "ExpansionFailed"} <p class="warning">Note: {secondary.error_message}</p> <p class="status-ok">Partial Calculation OK</p> {:else if secondary.error_message} <p class="warning">Note: {secondary.error_message}</p> <p class="status-error">Calculation Failed</p> {:else} <p class="status-ok">Calculation OK</p> {/if} {:else} <p class="status-error">Calculation Failed</p> {/if}
              <p><strong>Top-Level Action:</strong> {secondary.top_level_action ?? 'N/A'}</p>
              {#if !secondary.calculation_possible && secondary.error_message && secondary.top_level_action === "Unknown"}<p class="reason"><strong>Reason:</strong> {secondary.error_message}</p>{/if}
              <p><strong>Final Cost Method:</strong> {secondary.final_cost_method ?? 'N/A'} </p>
              <p><strong>Total Estimated Cost:</strong> <span class="cost">{formatNum(secondary.total_cost, 2)}</span></p>
              {#if secondary.calculation_possible}
                <p><strong>Price Per Unit (Acquired):</strong> {formatNum(secondarySummary.pricePerUnitCrafted, 2)}</p>
                <p><strong>Est. Profit Per Unit:</strong> {formatNum(secondarySummary.profitPerUnit, 2)} <i>(vs benchmark)</i></p>
              {/if}
              <p><i>(Benchmark Secondary Cost: {formatNum(secondary.top_level_cost, 2)})</i></p>
         </div>
      </div>

      <!-- Ingredient Tables: Reverted to <table> and placed in a single column flow -->
      <div class="ingredient-section">
          <!-- Table 1: Primary Based Ingredients -->
          <div class="results-table-container">
              <h3>Base Ingredients (Primary Perspective)</h3>
              {#if primaryIngredients.length > 0}
                  <p class="note">Ingredients derived from choosing the best C10M cost at each step. Total cost above reflects this optimal path.</p>
                  <div class="table-wrapper">
                      <table>
                          <thead>
                              <tr>
                                  <th>Base Item</th>
                                  <th class="num-col">Qty</th>
                                  <th class="num-col">Price/Unit (Assoc.)</th>
                                  <th class="num-col">Total Assoc. Cost</th>
                                  <th class="num-col">Optimal C10M Cost</th>
                                  <th>Method</th>
                                  <th class="num-col">RR</th>
                              </tr>
                          </thead>
                          <tbody>
                              {#each primaryIngredients as ing}
                                  <tr>
                                      <td>{ing.name}</td>
                                      <td class="num-col">{formatNum(ing.qty, 0)}</td>
                                      <td class="num-col">{formatNum(ing.pricePerUnit,2)}</td>
                                      <td class="num-col">{formatNum(ing.assocCost, 2)}</td>
                                      <td class="num-col cost-cell">{formatNum(ing.bestCost, 2)}</td>
                                      <td>{ing.method}</td>
                                      <td class="num-col">{formatNum(ing.rr, 2)}</td>
                                  </tr>
                              {/each}
                          </tbody>
                      </table>
                  </div>
              {:else if primary.calculation_possible && (primary.top_level_action === 'TreatedAsBase' || primary.top_level_action === 'TreatedAsBase (Due to Cycle)')}
                   <p>Top-level item treated as base for this perspective.</p>
              {:else if !loading}
                  <p>No base ingredients determined for this perspective.</p> {#if primary.error_message}<p class="error-detail">{primary.error_message}</p>{/if}
              {/if}
          </div>

          <!-- Table 2: Secondary Based Ingredients -->
           <div class="results-table-container">
              <h3>Base Ingredients (Secondary Perspective)</h3>
               {#if secondaryIngredients.length > 0}
                   <p class="note">Ingredients derived assuming the top-level item *must* be crafted (if possible). Total cost above is benchmarked differently.</p>
                   <div class="table-wrapper">
                      <table>
                          <thead>
                              <tr>
                                  <th>Base Item</th>
                                  <th class="num-col">Qty</th>
                                  <th class="num-col">Price/Unit (Assoc.)</th>
                                  <th class="num-col">Total Assoc. Cost</th>
                                  <th class="num-col">Optimal C10M Cost</th>
                                  <th>Method</th>
                                  <th class="num-col">RR</th>
                              </tr>
                          </thead>
                          <tbody>
                              {#each secondaryIngredients as ing}
                                  <tr>
                                      <td>{ing.name}</td>
                                      <td class="num-col">{formatNum(ing.qty, 0)}</td>
                                      <td class="num-col">{formatNum(ing.pricePerUnit,2)}</td>
                                      <td class="num-col">{formatNum(ing.assocCost, 2)}</td>
                                      <td class="num-col cost-cell">{formatNum(ing.bestCost, 2)}</td>
                                      <td>{ing.method}</td>
                                      <td class="num-col">{formatNum(ing.rr, 2)}</td>
                                  </tr>
                              {/each}
                          </tbody>
                      </table>
                   </div>
               {:else if secondary.calculation_possible && (secondary.top_level_action === 'TreatedAsBase' || secondary.top_level_action === 'TreatedAsBase (No Recipe)' || secondary.top_level_action === 'TreatedAsBase (Due to Cycle)')}
                  <p>Top-level item treated as base for this perspective.</p>
               {:else if !loading}
                   <p>No base ingredients determined for this perspective.</p> {#if secondary.error_message}<p class="error-detail">{secondary.error_message}</p>{/if}
               {/if}
          </div>
      </div>

  {:else if !loading && !error} <p>Enter an Item ID and Quantity.</p> {/if}
</main>

<style>
  main {
    max-width: 1000px; /* Slightly wider for better table display */
    margin: 1.5rem auto; /* Reduced top/bottom margin */
    padding: 1.5rem;
    background: #1e1e1e;
    border-radius: 8px;
    box-shadow: 0 2px 10px rgba(0,0,0,0.4);
    color: #ddd;
  }
  h1 { text-align: center; color: #fff; margin-bottom: 1.5rem; } /* Reduced margin */
  h2 { /* Perspective titles */
    margin-top: 0;
    font-size: 1.2rem; /* Slightly larger */
    color: #00aeff;
    border-bottom: 1px solid #444;
    padding-bottom: 0.5rem;
    margin-bottom: 1rem;
  }
  h3 { /* Table titles */
    margin-top: 0; /* Remove extra top margin if it's inside a container */
    margin-bottom: 0.6rem; /* Reduced margin */
    text-align: left;
    font-size: 1.05rem; /* Slightly smaller */
    color: #eee;
  }

  .controls { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 0.8rem; margin-bottom: 1.5rem; align-items: end; }
  label { display: flex; flex-direction: column; font-size: 0.85rem; color: #bbb;}
  input { padding: 0.5rem; border: 1px solid #555; border-radius: 4px; background: #2a2a2a; color: #fff; font-size: 0.95rem; margin-top: 0.2rem; }
  button { padding: 0.6rem 1rem; background: #0070f3; color: #fff; border: none; border-radius: 4px; font-size: 0.95rem; cursor: pointer; transition: background 0.2s; }
  button:hover:not(:disabled) { background: #005bb5; }
  button:disabled { background: #555; cursor: not-allowed; }

  .results-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1.2rem; margin-bottom: 1.5rem; } /* Grid for summary perspectives */
  .perspective { background: #26262a; padding: 1.2rem; border-radius: 6px; border: 1px solid #38383d; display: flex; flex-direction: column; }
  .perspective h2 { text-align: left; font-size: 1.05rem; border: none; padding: 0; }
  .perspective p { margin: 0.3rem 0; font-size: 0.85rem; line-height: 1.4; color: #ccc; }
  .perspective strong { color: #e0e0e0; min-width: 150px; display: inline-block;}
  .perspective .cost { font-weight: bold; font-size: 1rem; color: #50fa7b; }
  .perspective i { font-size: 0.75rem; color: #888; display: block; margin-top: 0.15rem; }
  .status-ok { color: #50fa7b; font-weight: bold; font-size: 0.8rem; text-transform: uppercase; }
  .status-error { color: #ff6b6b; font-weight: bold; font-size: 0.8rem; text-transform: uppercase; }
  .warning { font-size: 0.8rem; color: #f1fa8c; margin-top: 0.3rem; padding: 0.3rem 0.5rem; background-color: #44475a80; border-left: 2px solid #f1fa8c; border-radius: 3px; }
  .reason { font-size: 0.8rem !important; color: #aaa !important; }


  .ingredient-section { /* Container for both tables, will stack them */
    display: flex;
    flex-direction: column;
    gap: 1.5rem; /* Space between tables */
    margin-top: 1rem;
  }
  .results-table-container { /* Container for each table + its title/note */
      background: #252525;
      padding: 1rem 1.2rem; /* Reduced padding */
      border-radius: 6px;
      border: 1px solid #333;
  }
  .note { font-size: 0.8rem; color: #999; margin-bottom: 0.8rem; line-height: 1.3; font-style: italic; }

  .table-wrapper { /* This div will handle overflow if table is too wide */
      overflow-x: auto;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem; /* Slightly smaller table font */
  }
  th, td {
    padding: 0.5rem 0.7rem; /* Reduced padding */
    border: 1px solid #444;
    text-align: left;
    white-space: nowrap;
  }
  thead th {
    background: #2c2c2c;
    position: sticky; top: 0; /* Sticky header for vertical scroll within .table-wrapper if it had max-height */
    z-index: 1;
    font-size: 0.8rem; /* Smaller header font */
    color: #ccc;
  }
  tbody tr:nth-child(even) { background: #2a2a2a; }
  tbody tr:hover { background: #353535; }
  .num-col { text-align: right; } /* Class for right-aligning numbers */
  .cost-cell { color: #8be9fd; }

  .error { color: #ff6b6b; font-weight: bold; text-align: center; background: #442222; padding: 1rem; border-radius: 4px; border: 1px solid #ff6b6b; margin-top: 1rem; }
  .error-detail { color: #ff9a9a; font-size: 0.9rem; margin-top: 0.5rem; }
</style>