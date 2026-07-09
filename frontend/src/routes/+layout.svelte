<script>
  // Import global styles from src/app.css
  import "../app.css"; 
</script>

<svelte:head>
  <title>wiz - Your Bazaar Profit Wizard</title>
  <meta name="description" content="Maximize your bazaar trading profits with advanced analytics and AI-driven insights" />
  <!-- Import Inter font -->
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap" rel="stylesheet" />
</svelte:head>

<!-- Apply global font and background/text colors if not handled by Tailwind body styles in app.css -->
<div class="font-inter bg-dark text-light min-h-screen">
  <slot />
</div>

<style>
  /* 
    Global styles can also be defined here using :global(), 
    but for Tailwind setup, app.css is the standard place for directives.
  */
  /*
  :global(body) {
    background-color: #0B0B16; // Or use Tailwind class on the div above
    color: #e5e7eb; // Or use Tailwind class on the div above
  }
  */
</style>