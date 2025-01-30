<!-- src/BazaarItems.svelte -->
<script>
    import items from './bazaar_profitable_items.json';

    const formatNumber = (num) => {
        if (!num) return '0';
        return num.toLocaleString('en-US', { maximumFractionDigits: 0 });
    };
</script>

<style>
    .container {
        max-width: 1400px;
        margin: 0 auto;
        padding: 2rem;
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
    }

    .grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
        gap: 1.5rem;
        margin-top: 1rem;
    }

    .card {
        background: white;
        border-radius: 8px;
        padding: 1.5rem;
        box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        transition: transform 0.2s;
    }

    .card:hover {
        transform: translateY(-2px);
    }

    .item-header {
        border-bottom: 1px solid #eee;
        padding-bottom: 1rem;
        margin-bottom: 1rem;
    }

    .item-name {
        font-size: 1.2rem;
        font-weight: 600;
        color: #333;
        margin-bottom: 0.5rem;
    }

    .profit {
        color: #10B981;
        font-size: 1.4rem;
        font-weight: 700;
        margin-bottom: 0.5rem;
    }

    .stats {
        display: grid;
        gap: 0.5rem;
        font-size: 0.9rem;
        color: #666;
    }

    .ingredients {
        margin-top: 1rem;
        padding-top: 1rem;
        border-top: 1px solid #eee;
    }

    .ingredient {
        display: flex;
        justify-content: space-between;
        font-size: 0.85rem;
        padding: 0.3rem 0;
        color: #444;
    }

    .sell-method {
        display: inline-block;
        padding: 0.2rem 0.5rem;
        border-radius: 4px;
        font-size: 0.8rem;
        background: #E5E7EB;
        margin-top: 0.5rem;
    }

    .bazaar-buy { background: #BFDBFE; }
    .bazaar-sell { background: #BBF7D0; }
</style>

<div class="container">
    <h1>Bazaar Profitable Items</h1>
    <div class="grid">
        {#each items as item}
            <div class="card">
                <div class="item-header">
                    <div class="item-name">{item.item.replace(/_/g, ' ')}</div>
                    <div class="profit">⏣ {formatNumber(item.profit_per_hour)}/h</div>
                    <div class="stats">
                        <div>Sell Price: ⏣ {formatNumber(item.sell_price)}</div>
                        <div>Craft Cost: ⏣ {formatNumber(item.crafting_cost)}</div>
                        <div>Savings: ⏣ {formatNumber(item.crafting_savings)}</div>
                    </div>
                    <div class="sell-method {item.sell_method === 'Bazaar Instabuy' ? 'bazaar-sell' : 'bazaar-buy'}">
                        {item.sell_method}
                    </div>
                </div>

                <div class="ingredients">
                    <h3>Ingredients:</h3>
                    {#each item.ingredients as ingredient}
                        <div class="ingredient">
                            <span>{ingredient.item.replace(/_/g, ' ')}</span>
                            <span>
                                {ingredient.count_per_item}x @ ⏣{formatNumber(ingredient.cost_per_unit)}
                            </span>
                        </div>
                    {/each}
                </div>
            </div>
        {/each}
    </div>
</div>