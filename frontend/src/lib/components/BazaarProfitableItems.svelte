<script lang="ts">
  import { onMount } from 'svelte';

  // Define the shape of a bazaar item
  interface BazaarItem {
    item: string;
    profit_per_hour: number;
    crafting_cost: number;
    sell_price: number;
    cycles_per_hour: number;
    longest_step_count: number;
    crafting_savings: number;
  }

  let items: BazaarItem[] = [];
  let loading: boolean = true;
  let error: string | null = null;

  // Fetch top bazaar crafts on mount
  const fetchBazaarData = async (): Promise<void> => {
    try {
      const response = await fetch('/top_40_bazaar_crafts.json');
      if (!response.ok)
        throw new Error(`Bazaar Data Error: ${response.statusText}`);
      const data: BazaarItem[] = await response.json();
      items = data;
    } catch (err: unknown) {
      if (err instanceof Error) {
        error = `Bazaar Error: ${err.message}`;
      } else {
        error = 'Bazaar Error: Unknown error';
      }
    } finally {
      loading = false;
    }
  };

  onMount(fetchBazaarData);

  // Convert a raw item identifier (with underscores) to Title Case
  const toTitleCase = (str: string): string =>
    str
      .split('_')
      .map(
        (word) =>
          word.charAt(0).toUpperCase() + word.slice(1).toLowerCase()
      )
      .join(' ');

  // Abbreviate large numbers: 1,234 → 1.2K, 1,234,567 → 1.2M, etc.
  function abbreviateNumber(value: number): string {
    if (!value || isNaN(value)) return '0';
    const absValue = Math.abs(value);
    if (absValue >= 1.0e9) {
      return (value / 1.0e9)
        .toFixed(1)
        .replace(/\.0$/, '') + 'B';
    } else if (absValue >= 1.0e6) {
      return (value / 1.0e6)
        .toFixed(1)
        .replace(/\.0$/, '') + 'M';
    } else if (absValue >= 1.0e3) {
      return (value / 1.0e3)
        .toFixed(1)
        .replace(/\.0$/, '') + 'K';
    }
    return value.toString();
  }

  // Compute percent flip as (crafting_savings / crafting_cost) * 100
  const percentFlip = (item: BazaarItem): number => {
    if (!item.crafting_cost) return 0;
    return (item.crafting_savings / item.crafting_cost) * 100;
  };

  // Use green for flips greater than 5%, red for less than -5%, default otherwise.
  const flipColor = (pct: number): string => {
    if (pct > 5) return 'text-green-500';
    if (pct < -5) return 'text-red-500';
    return 'text-light';
  };
</script>

<style>
  :global(body) {
    margin: 0;
    background-color: #0B0B16; /* Use our dark color */
    color: #e5e7eb;
    font-family: system-ui, sans-serif;
  }

  .container {
    padding: 1.5rem;
    max-width: 1400px;
    margin: 0 auto;
  }

  .grid {
    display: grid;
    gap: 1.5rem;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  }

  .card {
    background: #1a1a1a;
    border-radius: 12px;
    padding: 1rem;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
    transition: transform 0.2s ease, box-shadow 0.2s ease;
    text-decoration: none;
    color: inherit;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
  }

  .card:hover {
    transform: translateY(-4px);
    box-shadow: 0 8px 12px rgba(0, 0, 0, 0.4);
  }

  .image-wrapper {
    margin-bottom: 1rem;
  }

  .image-wrapper img {
    width: 100%;
    height: 10rem;
    object-fit: contain;
    border-radius: 8px;
  }

  .item-info {
    text-align: center;
    margin-bottom: 1rem;
  }

  .item-name {
    margin-bottom: 0.25rem;
    font-size: 1.1rem;
    font-weight: 600;
  }

  .price {
    margin-bottom: 0.5rem;
    font-size: 1.2rem;
    font-weight: 700;
    color: #C8ACD6;
  }

  .stats {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .stats-line {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
  }

  .stat {
    display: flex;
    flex-direction: column;
    flex: 1;
    background: #12121E;
    border-radius: 6px;
    text-align: center;
    padding: 0.4rem 0;
  }

  .stat-value {
    font-size: 0.9rem;
    font-weight: 600;
    color: #e5e7eb;
  }
  .stat-sub {
    font-size: 0.7rem;
    color: #9ca3af;
    margin-top: 0.15rem;
  }
</style>

<div class="container">
  {#if loading}
    <div>🔄 Loading...</div>
  {:else if error}
    <div>❌ {error}</div>
  {:else}
    <div class="grid">
      {#each items as item (item.item)}
        <a class="card" href={`/bazaaritems/${encodeURIComponent(item.item)}`}>
          <!-- Image -->
          <div class="image-wrapper">
            <img 
              src={`https://sky.shiiyu.moe/item/${item.item}`}
              alt={toTitleCase(item.item)}
            />
          </div>

          <!-- Centered item info -->
          <div class="item-info">
            <div class="item-name">{toTitleCase(item.item)}</div>
            <div class="price">{abbreviateNumber(item.profit_per_hour)}/h</div>
          </div>

          <!-- Stats -->
          <div class="stats">
            <div class="stats-line">
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(item.crafting_cost)}</span>
                <span class="stat-sub">craft</span>
              </div>
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(item.sell_price)}</span>
                <span class="stat-sub">sell</span>
              </div>
            </div>
            <div class="stats-line">
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(item.cycles_per_hour)}</span>
                <span class="stat-sub">cycles/h</span>
              </div>
              <div class="stat">
                <span class="stat-value">{item.longest_step_count}</span>
                <span class="stat-sub">depth</span>
              </div>
            </div>
            <div class="stats-line">
              <div class="stat">
                <span class="stat-value">{abbreviateNumber(item.crafting_savings)}</span>
                <span class="stat-sub">savings</span>
              </div>
              <div class="stat">
                <!-- Color determined based on flip percentage -->
                <span class={`stat-value ${flipColor(percentFlip(item))}`}>
                  {percentFlip(item).toFixed(2)}%
                </span>
                <span class="stat-sub">flip</span>
              </div>
            </div>
          </div>
        </a>
      {/each}
    </div>
  {/if}
</div>
