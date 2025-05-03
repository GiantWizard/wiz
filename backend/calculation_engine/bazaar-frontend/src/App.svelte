<script>
  import { onMount } from 'svelte';

  let item = 'DIAMOND';
  let qty = 1;
  let result = null;
  let error = null;

  async function calculate() {
    console.log('🔍 calculate()', { item, qty });
    error = null;
    result = null;

    // MUST call /api/fill so Vite will proxy to Go
    const params = new URLSearchParams({ item, qty: String(qty) });
    try {
      const res = await fetch(`/api/fill?${params}`);
      if (!res.ok) throw new Error(await res.text());
      result = await res.json();
    } catch (e) {
      error = e.message;
    }
  }

  onMount(calculate);
</script>

<main>
  <h1>Bazaar Fill‑Time Dashboard</h1>

  <div class="controls">
    <label>
      Item ID:
      <input bind:value={item} placeholder="e.g. DIAMOND" />
    </label>
    <label>
      Quantity:
      <input type="number" min="0" bind:value={qty} />
    </label>
    <button on:click={calculate}>Calculate</button>
  </div>

  {#if error}
    <p class="error">Error: {error}</p>
  {:else if result}
    <div class="results">
      <h2>Recipe Breakdown</h2>
      <div class="scroll">
        <table>
          <thead>
            <tr>
              <th>Item</th>
              <th>Quantity</th>
              <th>Instasell Fill (s)</th>
              <th>Buy‑Order Fill (s)</th>
              <th>RR</th>
            </tr>
          </thead>
          <tbody>
            {#each result.recipe as ing}
              <tr>
                <td>{ing.name}</td>
                <td>{ing.qty}</td>
                <td>{ing.instasell_fill_time.toFixed(2)}</td>
                <td>{ing.buy_order_fill_time.toFixed(2)}</td>
                <td>{ing.rr}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="summary">
        <p><strong>Slowest ingredient:</strong> {result.slowest_ingredient} (x{result.slowest_ingredient_qty})</p>
        <p><strong>Total fill time:</strong> {result.slowest_fill_time.toFixed(2)} s</p>
      </div>
    </div>
  {/if}
</main>

<style>
  :global(body) {
    margin: 0;
    padding: 0;
    background: #121212;
    color: #e0e0e0;
    font-family: sans-serif;
  }

  main {
    max-width: 800px;
    margin: 2rem auto;
    padding: 1.5rem;
    background: #1e1e1e;
    border-radius: 8px;
    box-shadow: 0 2px 10px rgba(0,0,0,0.4);
  }

  h1, h2 {
    text-align: center;
    color: #ffffff;
  }

  .controls {
    display: grid;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }

  label {
    display: flex;
    flex-direction: column;
    font-size: 0.9rem;
  }

  input {
    padding: 0.5rem;
    border: 1px solid #555;
    border-radius: 4px;
    background: #2a2a2a;
    color: #fff;
    font-size: 1rem;
  }

  button {
    padding: 0.6rem 1.2rem;
    background: #0070f3;
    color: white;
    border: none;
    border-radius: 4px;
    font-size: 1rem;
    cursor: pointer;
    transition: background 0.2s;
  }
  button:hover {
    background: #005bb5;
  }

  .results .scroll {
    overflow-x: auto;
    margin-bottom: 1rem;
  }

  table {
    width: 100%;
    border-collapse: collapse;
  }

  th, td {
    padding: 0.75rem;
    border: 1px solid #444;
    text-align: left;
  }

  th {
    background: #2c2c2c;
  }

  tr:nth-child(even) {
    background: #1a1a1a;
  }

  .summary p {
    font-size: 1rem;
    margin: 0.5rem 0;
  }

  .error {
    color: #ff5555;
    font-weight: bold;
    text-align: center;
  }
</style>
