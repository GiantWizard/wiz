<script>
    import { onMount } from 'svelte';

    let bazaarItems = [];
    let loading = true;
    let error = null;

    const fetchBazaarData = async () => {
        try {
            const response = await fetch('/bazaar_profitable_items.json');
            if (!response.ok) throw new Error(`Bazaar Data Error: ${response.statusText}`);
            bazaarItems = await response.json();
        } catch (err) {
            error = `Bazaar Error: ${err.message}`;
        } finally {
            loading = false;
        }
    };

    onMount(fetchBazaarData);

    const formatNumber = (num) => num?.toLocaleString('en-US') || '0';
</script>

<div class="container">
    <h1>Bazaar Profits</h1>
    {#if loading}
        <p>Loading Bazaar Data...</p>
    {:else if error}
        <p class="error">{error}</p>
    {:else}
        <div class="grid">
            {#each bazaarItems as item}
                <div class="card bazaar-card">
                    <div class="item-header">
                        <div class="item-name">{item.item.replace(/_/g, ' ')}</div>
                        <div class="profit">⏣ {formatNumber(item.profit_per_hour)}/h</div>
                        <div class="stats">
                            <div>Buy Price: ⏣ {formatNumber(item.buy_price)}</div>
                            <div>Sell Price: ⏣ {formatNumber(item.sell_price)}</div>
                            <div>Velocity: {formatNumber(item.demand)}/h</div>
                        </div>
                    </div>
                </div>
            {/each}
        </div>
    {/if}
</div>

<style>
    .bazaar-card {
        border-left: 4px solid #10B981;
    }

    .error {
        color: #EF4444;
        padding: 1rem;
        background: #FEE2E2;
        border-radius: 4px;
    }
</style>