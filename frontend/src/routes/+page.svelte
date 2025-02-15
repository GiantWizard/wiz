<script>
  import { goto } from '$app/navigation';
</script>

<style>
  .hero {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 4rem 1rem;
    background: linear-gradient(135deg, #10B981, #06b6d4);
    color: white;
    text-align: center;
    min-height: 100vh;
  }

  h1 {
    font-size: 3rem;
    margin-bottom: 1rem;
  }

  p {
    font-size: 1.5rem;
    margin-bottom: 2rem;
  }

  .btn {
    background-color: white;
    color: #10B981;
    border: none;
    padding: 1rem 2rem;
    font-size: 1.2rem;
    border-radius: 8px;
    cursor: pointer;
    transition: transform 0.2s ease, box-shadow 0.2s ease;
  }

  .btn:hover {
    transform: translateY(-3px);
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.2);
  }
</style>

<div class="hero">
  <h1>Welcome to the Bazaar Profit Tracker</h1>
  <p>Select an option below to view detailed crafting recipes and profitability metrics.</p>
  <button class="btn" on:click={() => goto('/bazaaritems')}>
    View Bazaar Profitable Items
  </button>
</div>
