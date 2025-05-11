<!-- src/OptimizerPage.svelte -->
<script>
    import { onMount } from 'svelte';
  
    let optimizationResults = [];
    let error = null;
    let loading = false;
    let timeLimitSecs = 3600; // Default 1 hour
    let maxQtySearch = 1000000; // Default 1 million
  
    async function runOptimization() {
      error = null;
      optimizationResults = [];
      loading = true;
  
      const params = new URLSearchParams();
      if (timeLimitSecs && timeLimitSecs > 0) {
          params.append('time_limit_secs', String(timeLimitSecs));
      }
      if (maxQtySearch && maxQtySearch > 0) {
          params.append('max_qty_search', String(maxQtySearch));
      }
  
      try {
        console.log(`Fetching: /api/optimize-all?${params}`);
        const res = await fetch(`/api/optimize-all?${params}`);
        if (!res.ok) {
          const errorText = await res.text();
          console.error(`API Error ${res.status}: ${errorText}`);
          throw new Error(`Optimizer API Error ${res.status}: ${errorText || res.statusText}`);
        }
        const data = await res.json();
        console.log("Optimizer API Results:", data);
        if (Array.isArray(data)) {
          optimizationResults = data;
        } else {
          // Check if it's a single error object (as per optimizer.go Batch Error)
          if (data && data.item_name === "BATCH_ERROR" && data.error_message) {
              throw new Error(`Batch Optimization Error: ${data.error_message}`);
          }
          throw new Error("Unexpected API response format. Expected an array of optimization results.");
        }
      } catch (e) {
        console.error("Fetch/Optimization Error:", e);
        error = e.message || 'An unknown error occurred during optimization.';
      } finally {
        loading = false;
      }
    }
  
    function formatNum(num, digits = 2) {
        if (num === null || num === undefined) return 'N/A';
        if (isNaN(num)) return 'N/A';
        if (!isFinite(num)) return num > 0 ? 'Infinite' : num < 0 ? '-Infinite' : 'N/A';
        return Number(num).toFixed(digits);
    }
  
    function formatTime(num) {
         if (num === null || num === undefined) return 'N/A'; 
         if (isNaN(num)) return 'N/A'; 
         if (!isFinite(num)) return num > 0 ? 'Infinite' : 'N/A (-Inf)';
         if (num < 0) return 'N/A (<0)';
         if (num === 0) return '0.0s';
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
    <h1>Bazaar Crafting Optimizer</h1>
  
    <div class="controls">
      <label>Max Total Cycle Time (seconds):
        <input type="number" min="60" step="60" bind:value={timeLimitSecs} disabled={loading} />
      </label>
      <label>Max Initial Qty Search:
        <input type="number" min="1000" step="1000" bind:value={maxQtySearch} disabled={loading} />
      </label>
      <button on:click={runOptimization} disabled={loading}>
        {#if loading}Running Optimization...{:else}Run Full Optimization{/if}
      </button>
    </div>
  
    {#if error}
      <p class="error-message">Error: {error}</p>
    {/if}
  
    {#if !loading && optimizationResults.length > 0}
      <p class="page-note">Displaying top profitable items. Times are: Acquisition (P1 ingredients/direct), Sale (instasell final), Total (sum used for constraint).</p>
      <div class="table-wrapper">
        <table>
          <thead>
            <tr>
              <th>#</th>
              <th>Item Name</th>
              <th class="num-col">Max Profit</th>
              <th class="num-col">Opt. Qty</th>
              <th class="num-col">Cost @ Qty</th>
              <th class="num-col">Revenue @ Qty</th>
              <th class="num-col">Acquisition Time</th>
              <th class="num-col">Sale Time</th>
              <th class="num-col">Total Cycle Time</th>
              <th>Bottleneck Ing.</th>
              <th class="num-col">Bottleneck Qty</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {#each optimizationResults as result, i}
              <tr class:not-possible={!result.calculation_possible}>
                <td>{i + 1}</td>
                <td>{result.item_name}</td>
                <td class="num-col profit-cell">{formatNum(result.max_profit)}</td>
                <td class="num-col">{formatNum(result.max_feasible_quantity, 0)}</td>
                <td class="num-col">{formatNum(result.cost_at_optimal_qty)}</td>
                <td class="num-col">{formatNum(result.revenue_at_optimal_qty)}</td>
                <td class="num-col">{formatTime(result.acquisition_time_at_optimal_qty)}</td>
                <td class="num-col">{formatTime(result.sale_time_at_optimal_qty)}</td>
                <td class="num-col time-highlight">{formatTime(result.total_cycle_time_at_optimal_qty)}</td>
                <td>{result.bottleneck_ingredient || 'N/A'}</td>
                <td class="num-col">{formatNum(result.bottleneck_ingredient_qty, 0)}</td>
                <td>
                  {#if result.calculation_possible}
                    <span class="status-ok">OK</span>
                  {:else}
                    <span class="status-error" title={result.error_message}>Failed</span>
                     {#if result.error_message} <span class="error-tooltip-trigger">?</span> {/if}
                  {/if}
                </td>
              </tr>
              {#if !result.calculation_possible && result.error_message}
                <tr class="error-detail-row">
                  <td colspan="11" class="error-detail-msg">{result.error_message}</td> <!-- Adjusted colspan -->
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    {:else if !loading && !error}
      <p class="page-note">Click "Run Full Optimization" to start. This may take some time depending on the number of items and server load.</p>
    {/if}
  </main>
  
  <style>
    main { max-width: 1300px; margin: 1.5rem auto; padding: 1.5rem; background: #1e1e1e; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.4); color: #ddd; }
    h1 { text-align: center; color: #fff; margin-bottom: 1.5rem; }
    .page-note { 
      text-align: center;
      margin-bottom: 1.5rem;
      font-style: italic;
      color: #aaa;
    }
    .controls { display: flex; flex-wrap: wrap; gap: 1rem; margin-bottom: 1.5rem; padding: 1rem; background: #2a2a2a; border-radius: 6px; align-items: flex-end; }
    .controls label { display: flex; flex-direction: column; font-size: 0.85rem; color: #bbb; min-width: 200px; flex-grow: 1;}
    .controls input { padding: 0.5rem; border: 1px solid #555; border-radius: 4px; background: #252525; color: #fff; font-size: 0.95rem; margin-top: 0.2rem; width: 100%; box-sizing: border-box; }
    .controls button { padding: 0.6rem 1rem; background: #0070f3; color: #fff; border: none; border-radius: 4px; font-size: 0.95rem; cursor: pointer; transition: background 0.2s; height: fit-content; margin-left: auto; }
    .controls button:hover:not(:disabled) { background: #005bb5; }
    .controls button:disabled { background: #555; cursor: not-allowed; }
  
    .table-wrapper { overflow-x: auto; margin-top: 1rem; border: 1px solid #333; border-radius: 6px; }
    table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
    th, td { padding: 0.6rem 0.8rem; border-bottom: 1px solid #444; border-left: 1px solid #444; text-align: left; white-space: nowrap; }
    thead th { background: #2c2c2c; position: sticky; top: 0; z-index: 1; font-size: 0.8rem; color: #ccc; border-top: none;}
    td:first-child, th:first-child { border-left: none; }
    tbody tr:nth-child(even) { background: #26262a; }
    tbody tr:hover { background: #353535; }
    .num-col { text-align: right; }
    .profit-cell { font-weight: bold; color: #50fa7b; }
    .time-highlight { font-weight: bold; color: #8be9fd; } /* Light blue for total cycle time */
    .not-possible { color: #777; } 
    .not-possible .profit-cell { color: #ff8a8a; }
    .not-possible .time-highlight { color: #aaa; font-weight: normal; }
    .status-ok { color: #50fa7b; font-weight: bold;}
    .status-error { color: #ff6b6b; font-weight: bold;}
  
    .error-tooltip-trigger {
      display: inline-block;
      margin-left: 5px;
      color: #f1fa8c;
      cursor: help;
      border: 1px solid #f1fa8c;
      border-radius: 50%;
      width: 14px;
      height: 14px;
      font-size: 10px;
      line-height: 14px;
      text-align: center;
    }
    .error-detail-row td {
      padding: 0.4rem 0.8rem;
      font-size: 0.8rem;
      color: #bbb;
      background-color: #3a3030 !important; 
      white-space: normal;
      border-bottom: 1px dashed #555; 
    }
    .error-detail-row:hover td {
       background-color: #423535 !important;
    }
  
    .error-message { color: #ff6b6b; font-weight: bold; text-align: center; background: #442222; padding: 1rem; border-radius: 4px; border: 1px solid #ff6b6b; margin: 1rem; }
  </style>