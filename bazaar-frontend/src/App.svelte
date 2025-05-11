<!-- src/App.svelte -->
<script>
    import { onMount, onDestroy } from 'svelte';
    import DualExpansionPage from './DualExpansionPage.svelte';
    import OptimizerPage from './OptimizerPage.svelte';
  
    let currentPageComponent = null;
  
    function handleHashChange() {
      const hash = window.location.hash;
      console.log("Current Hash:", hash);
  
      if (hash === '#/optimize') {
        currentPageComponent = OptimizerPage;
      } else if (hash === '#/expand') {
        currentPageComponent = DualExpansionPage;
      } else {
        console.log("Defaulting or unrecognized hash, navigating to #/expand");
        window.location.hash = '#/expand';
      }
    }
  
    onMount(() => {
      console.log("App.svelte (Router) onMount");
      window.addEventListener('hashchange', handleHashChange);
      handleHashChange();
    });
  
    onDestroy(() => {
      window.removeEventListener('hashchange', handleHashChange);
    });
  </script>
  
  <header class="app-header">
    <nav>
      <a href="#/expand" class:active={currentPageComponent === DualExpansionPage}>Dual Expansion Calc</a>
      <a href="#/optimize" class:active={currentPageComponent === OptimizerPage}>Optimizer</a>
    </nav>
  </header>
  
  <div class="page-content">
    {#if currentPageComponent}
      <svelte:component this={currentPageComponent} />
    {:else}
      <p>Loading page...</p>
    {/if}
  </div>
  
  <style>
    :global(body) {
      margin: 0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen,
        Ubuntu, Cantarell, 'Open Sans', 'Helvetica Neue', sans-serif;
      background-color: #121212;
      color: #e0e0e0;
    }
  
    .app-header {
      background-color: #1e1e1e;
      padding: 0.5em 1em;
      box-shadow: 0 2px 5px rgba(0,0,0,0.3);
      border-bottom: 1px solid #333;
    }
  
    nav {
      display: flex;
      justify-content: center;
      align-items: center;
      max-width: 1200px;
      margin: 0 auto;
    }
  
    nav a {
      color: #00aeff;
      margin: 0 1.5em;
      padding: 0.8em 0.5em;
      text-decoration: none;
      font-weight: 500;
      border-bottom: 2px solid transparent;
      transition: color 0.2s ease, border-bottom-color 0.2s ease;
    }
  
    nav a:hover {
      color: #61dafb;
      border-bottom-color: #61dafb;
    }
    nav a.active {
       color: #fff;
       border-bottom-color: #00aeff;
       font-weight: bold;
    }
    .page-content {
       padding: 1em;
    }
  </style>