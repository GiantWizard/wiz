<script>
    import { goto } from '$app/navigation';
</script>

<h1>Welcome to the Bazaar Profit Tracker</h1>

<p>Select an option below:</p>

<button on:click={() => goto('/bazaaritems')}>View Bazaar Profitable Items</button>
<button on:click={() => goto('/allitems')}>View Auction House Profitable Items</button>
