<script>
  // Import global styles
  import "../app/global.css";
</script>

<svelte:head>
  <title>wiz - Your Bazaar Profit Wizard</title>
  <meta name="description" content="Maximize your bazaar trading profits with advanced analytics and AI-driven insights" />
  <!-- Import Inter font -->
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=Inter&display=swap" rel="stylesheet" />
</svelte:head>

<!-- Wrap page content in a div to apply the Inter font -->
<div class="font-inter">
  <slot />
</div>
