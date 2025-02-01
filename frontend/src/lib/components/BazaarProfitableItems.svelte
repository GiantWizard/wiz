<!-- src/lib/components/BazaarProfitableItems.svelte -->
<script>
    export let items = [];
    export let loading = true;
    export let error = null;

    const formatNumber = (num, decimals = 1) => {
        if (!num) return '0';
        const formatted = num.toLocaleString('en-US', {
            minimumFractionDigits: decimals,
            maximumFractionDigits: decimals
        });
        return formatted.replace(/.0$/, ''); // Remove trailing .0
    };

    // Aggregate similar ingredients
    const aggregateIngredients = (ingredients) => {
        const map = new Map();
        ingredients.forEach(ing => {
            const key = `${ing.item}-${ing.cost_per_unit}`;
            if (map.has(key)) {
                map.get(key).count += ing.count_per_item;
            } else {
                map.set(key, {
                    item: ing.item,
                    count: ing.count_per_item,
                    cost: ing.cost_per_unit
                });
            }
        });
        return Array.from(map.values());
    };
</script>

<style>
    .container {
        padding: 1rem;
        max-width: 1400px;
        margin: 0 auto;
    }

    .grid {
        display: grid;
        gap: 1rem;
        grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
    }

    .card {
        background: white;
        border-radius: 8px;
        padding: 1rem;
        box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        border-left: 3px solid #10B981;
        font-size: 0.9em;
    }

    .header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: 0.5rem;
    }

    .item-name {
        font-weight: 600;
        color: #1F2937;
        font-size: 1.1em;
    }

    .profit {
        color: #059669;
        font-weight: 700;
        font-size: 1em;
        white-space: nowrap;
    }

    .stats-grid {
        display: grid;
        grid-template-columns: repeat(2, 1fr);
        gap: 0.5rem;
        margin-bottom: 0.5rem;
    }

    .stat {
        display: flex;
        justify-content: space-between;
        padding: 0.3rem 0;
        border-bottom: 1px solid #F3F4F6;
    }

    .stat-label {
        color: #6B7280;
    }

    .stat-value {
        color: #1F2937;
        font-weight: 500;
    }

    .ingredients {
        margin-top: 0.5rem;
        padding-top: 0.5rem;
    }

    .ingredient {
        display: flex;
        justify-content: space-between;
        font-size: 0.85em;
        padding: 0.2rem 0;
    }

    .ingredient-name {
        color: #4B5563;
        margin-right: 0.5rem;
    }

    .ingredient-details {
        color: #6B7280;
        white-space: nowrap;
    }
</style>

<div class="container">
    {#if loading}
        <div class="loading">🔄 Loading...</div>
    {:else if error}
        <div class="error">❌ {error}</div>
    {:else}
        <div class="grid">
            {#each items as item}
                <div class="card">
                    <div class="header">
                        <div class="item-name">{item.item.replace(/_/g, ' ')}</div>
                        <div class="profit">⏣ {formatNumber(item.profit_per_hour)}/h</div>
                    </div>

                    <div class="stats-grid">
                        <div class="stat">
                            <span class="stat-label">Craft Cost</span>
                            <span class="stat-value">⏣ {formatNumber(item.crafting_cost)}</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Sell Price</span>
                            <span class="stat-value">⏣ {formatNumber(item.sell_price)}</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Velocity</span>
                            <span class="stat-value">{formatNumber(item.fill_time)}/h</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Savings</span>
                            <span class="stat-value" style="color: #059669">
                                ▲ {formatNumber(item.crafting_savings)}
                            </span>
                        </div>
                    </div>

                    {#if item.ingredients.length > 0}
                        <div class="ingredients">
                            {#each aggregateIngredients(item.ingredients) as ing}
                                <div class="ingredient">
                                    <span class="ingredient-name">
                                        {formatNumber(ing.count, 0)}x {ing.item.replace(/_/g, ' ')}
                                    </span>
                                    <span class="ingredient-details">
                                        @ ⏣ {formatNumber(ing.cost)}
                                    </span>
                                </div>
                            {/each}
                        </div>
                    {/if}
                </div>
            {/each}
        </div>
    {/if}
</div>