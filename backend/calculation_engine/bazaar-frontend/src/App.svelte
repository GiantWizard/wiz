<!-- This is conceptual Svelte code - Place this logic in your actual App.svelte file -->
<script>
  import { onMount } from 'svelte';

  let item = 'DIAMOND'; // Default item
  let qty = 1;          // Default quantity
  let result = null;
  let error = null;
  let loading = false;

  async function calculate() {
    error = null;
    result = null;
    loading = true; // Indicate loading state
    const normalizedItem = item.toUpperCase().trim();
    if (!normalizedItem) { error = "Item ID cannot be empty."; loading = false; return; }
    const params = new URLSearchParams({ item: normalizedItem, qty: String(qty) });
    try {
      console.log(`Fetching: /api/fill?${params}`);
      const res = await fetch(`/api/fill?${params}`);
      if (!res.ok) {
          const errorText = await res.text();
          console.error(`API Error ${res.status}: ${errorText}`);
          throw new Error(`API Error ${res.status}: ${errorText || res.statusText}`);
      }
      result = await res.json();
      console.log("API Result:", result);
    } catch (e) {
      console.error("Fetch/Calculation Error:", e);
      error = e.message || 'An unknown error occurred.';
    } finally {
      loading = false; // Turn off loading state
    }
  }

  onMount(calculate);

  function handleClick() { if (!loading) { calculate(); } }

  function formatNum(num, digits = 2) {
      if (num === null || num === undefined || isNaN(num)) return 'N/A';
      if (!isFinite(num)) return num > 0 ? 'Infinite' : '-Infinite';
      // Use sanitizeFloat(0.0) on backend, so check for 0 if it means N/A
      // Or check for null directly if using interface{} -> null mapping
      // Sticking with sanitizeFloat(0.0) means 0 is ambiguous, but simpler JSON
      return Number(num).toFixed(digits);
  }

  function formatTime(num) {
       if (num === null || num === undefined || isNaN(num)) return 'N/A';
       // Use sanitizeFloat(0.0) on backend, 0 now means NaN/Inf or actual zero.
       // Cannot distinguish here easily without backend sending null.
       // Displaying 0.0s might be acceptable compromise.
       if (!isFinite(num)) return num > 0 ? 'Infinite' : 'N/A (-Inf)';
       if (num < 0) return 'N/A (<0)';
       if (num === 0) return '0.0s'; // Could mean actual 0 or NaN/Inf from backend
       if (num < 1) return num.toFixed(2) + 's';
       if (num < 60) return num.toFixed(1) + 's';
       let mins = num / 60;
       if (mins < 60) return mins.toFixed(1) + 'm';
       let hours = mins / 60;
       if (hours < 24) return hours.toFixed(1) + 'h';
       let days = hours / 24;
       return days.toFixed(1) + 'd';
  }
</script>

<main>
  <h1>Bazaar Fill‑Time Dashboard</h1>

  <div class="controls">
    <label>Item ID:
      <input bind:value={item} placeholder="e.g. DIAMOND" disabled={loading} on:keyup={(e) => e.key === 'Enter' && handleClick()} />
    </label>
    <label>Quantity:
      <input type="number" min="1" step="1" bind:value={qty} disabled={loading} on:keyup={(e) => e.key === 'Enter' && handleClick()} />
    </label>
    <button on:click={handleClick} disabled={loading}>
      {#if loading}Calculating...{:else}Calculate{/if}
    </button>
  </div>

  {#if error}
    <p class="error">Error: {error}</p>
  {/if}

  {#if result && !error}
      {#if result.recipe && result.recipe.length > 0}
      <div class="results">
      <h2>Recipe Breakdown for {qty} x {item.toUpperCase().trim()}</h2>
      <div class="scroll">
          <table>
          <thead>
              <tr>
                <th>Item</th>
                <th>Qty</th>
                <th>Cost/Unit</th>
                <th>Total Cost</th>
                <th>Cost Src</th>
                <!-- <<< MODIFICATION: Removed Instasell Time Header >>> -->
                <th>Buy Order Time</th>
                <th>RR</th>
              </tr>
          </thead>
          <tbody>
              {#each result.recipe as ing}
              <tr>
                  <td>{ing.name}</td>
                  <td>{formatNum(ing.qty, 0)}</td>
                  <td>{formatNum(ing.cost_per_unit, 2)}</td>
                  <td>{formatNum(ing.total_cost, 2)}</td>
                  <td>{ing.price_source}</td>
                  <!-- <<< MODIFICATION: Removed Instasell Time Cell >>> -->
                  <td>{formatTime(ing.buy_order_fill_time)}</td>
                  <td>{formatNum(ing.rr, 2)}</td>
              </tr>
              {/each}
          </tbody>
          </table>
      </div>

      <div class="summary-section">
          <h3>Profit Summary</h3>
          <p><strong>Total Base Cost:</strong> {formatNum(result.total_base_cost, 2)}</p>
          <p><strong>Est. Sell Price (Unit):</strong> {formatNum(result.top_sell_price, 2)}</p>
          <p><strong>Est. Total Revenue:</strong> {formatNum(result.total_revenue, 2)}</p>
          <p><strong>Est. Profit per Unit:</strong> {formatNum(result.profit_per_unit, 2)}</p>
          <p><strong>Est. Total Profit:</strong> {formatNum(result.total_profit, 2)}</p>
      </div>

      <div class="summary-section">
          <h3>Fill Time Summary</h3>
           {#if result.slowest_ingredient}
               <p><strong>Slowest Ingredient (Buy Order):</strong> {result.slowest_ingredient} (x{formatNum(result.slowest_ingredient_qty, 0)})</p>
               <p><strong>Est. Total Buy Order Fill Time:</strong> {formatTime(result.slowest_fill_time)}</p>
            {:else if result.recipe.length > 0}
                 <p>Est. Total Buy Order Fill Time: 0.0s (or N/A)</p>
           {/if}
           <!-- <<< MODIFICATION: Added Top-Level Times >>> -->
           <p><strong>Est. Top-Level Instasell Time:</strong> {formatTime(result.top_level_instasell_time)}</p>
           <p><strong>Est. Top-Level Sell Order Time:</strong> {formatTime(result.top_level_sell_order_time)}</p>
      </div>
      </div>
      {:else}
          <p>No recipe breakdown available for {item.toUpperCase().trim()}. It might be a base item or expansion failed.</p>
      {/if}
  {:else if !loading && !error}
      <p>Enter an Item ID and Quantity.</p>
  {/if}
</main>

<style>
  /* Styles remain the same as before */
  main { max-width: 950px; margin: 2rem auto; padding: 1.5rem; background: #1e1e1e; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.4); }
  h1, h2, h3 { text-align: center; color: #fff; margin-bottom: 1rem;}
  h2 { margin-top: 2rem; }
  h3 { margin-top: 1.5rem; border-bottom: 1px solid #444; padding-bottom: 0.5rem;}
  .controls { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 1.5rem; align-items: end; }
  label { display: flex; flex-direction: column; font-size: 0.9rem; color: #bbb;}
  input { padding: 0.6rem; border: 1px solid #555; border-radius: 4px; background: #2a2a2a; color: #fff; font-size: 1rem; margin-top: 0.25rem; }
  input[type=number] { -moz-appearance: textfield; }
  input[type=number]::-webkit-outer-spin-button,
  input[type=number]::-webkit-inner-spin-button { -webkit-appearance: none; margin: 0; }
  button { padding: 0.7rem 1.2rem; background: #0070f3; color: #fff; border: none; border-radius: 4px; font-size: 1rem; cursor: pointer; transition: background 0.2s; }
  button:hover:not(:disabled) { background: #005bb5; }
  button:disabled { background: #555; cursor: not-allowed; }
  .results { margin-top: 2rem; }
  .scroll { overflow-x: auto; margin-bottom: 1rem; border: 1px solid #333; border-radius: 4px; }
  table { width: 100%; border-collapse: collapse; }
  th, td { padding: 0.75rem 1rem; border: 1px solid #444; text-align: left; white-space: nowrap; }
  thead th { background: #2c2c2c; position: sticky; top: 0; z-index: 1; }
  tbody tr:nth-child(even) { background: #242424; }
  tbody tr:hover { background: #303030; }
  .summary-section { background: #2a2a2a; padding: 1rem 1.5rem; margin-top: 1rem; border-radius: 4px; border: 1px solid #333; }
  .summary-section p { margin: 0.4rem 0; font-size: 1rem; }
  .summary-section strong { color: #ccc; min-width: 150px; display: inline-block;}
  .error { color: #ff6b6b; font-weight: bold; text-align: center; background: #442222; padding: 1rem; border-radius: 4px; border: 1px solid #ff6b6b; }
</style>