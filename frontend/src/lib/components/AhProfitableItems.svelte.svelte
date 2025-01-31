<script>
    import { onMount } from 'svelte';

    let ahItems = [];
    let loading = true;
    let error = null;

    const fetchAhData = async () => {
        try {
            const response = await fetch('/ah_included_profitable_items.json');
            if (!response.ok) throw new Error(`AH Data Error: ${response.statusText}`);
            ahItems = await response.json();
        } catch (err) {
            error = `Auction House Error: ${err.message}`;
        } finally {
            loading = false;
        }
    };

    onMount(fetchAhData);

    const formatNumber = (num) => num?.toLocaleString('en-US') || '0';
</script>

<div class="container">
    <h1>Auction House Profits</h1>
    {#if loading}
        <p>Loading AH Data...</p>
    {:else if error}
        <p class="error">{error}</p>
    {:else}
        <div class="grid">
            {#each ahItems as item}
                <div class="card ah-card">
                    <div class="item-header">
                        <div class="item-name">{item.item.replace(/_/g, ' ')}</div>
                        <div class="profit">⏣ {formatNumber(item.profit_per_hour)}/h</div>
                        <div class="stats">
                            <div>AH Price: ⏣ {formatNumber(item.sell_price)}</div>
                            <div>Craft Cost: ⏣ {formatNumber(item.buy_price)}</div>
                            <div>Volume: {formatNumber(item.market_volume)}/day</div>
                        </div>
                    </div>
                </div>
            {/each}
        </div>
    {/if}
</div>

<style>
    .ah-card {
        border-left: 4px solid #3B82F6;
    }

    .error {
        color: #EF4444;
        padding: 1rem;
        background: #FEE2E2;
        border-radius: 4px;
    }
</style>