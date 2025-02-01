<script>
    import { onMount } from 'svelte';
    import BazaarProfitableItems from '$lib/components/BazaarProfitableItems.svelte';

    let bazaarData = {
        items: [],
        loading: true,
        error: null
    };

    const fetchBazaarData = async () => {
        try {
            const response = await fetch('/bazaar_profitable_items.json');
            if (!response.ok) throw new Error(`Bazaar Data Error: ${response.statusText}`);
            bazaarData.items = await response.json();
        } catch (err) {
            bazaarData.error = `Bazaar Error: ${err.message}`;
        } finally {
            bazaarData.loading = false;
        }
    };

    onMount(fetchBazaarData);
</script>

<BazaarProfitableItems 
    items={bazaarData.items}
    loading={bazaarData.loading}
    error={bazaarData.error}
/>