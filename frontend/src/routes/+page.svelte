<script lang="ts">
  import { onMount } from 'svelte';
  import BazaarProfitableItems from '$lib/components/BazaarProfitableItems.svelte';
  import { DollarSign, TrendingUp, Calendar, PieChart, BarChart2, AlertTriangle, Zap, Target, UserCheck2 as PersonStanding } from 'lucide-svelte';
  import type { KoyebOptimizerResponse, KoyebFailedItemsReportResponse, ApiErrorResponse } from '$lib/types/koyeb';

  // --- Data for landing page sections (remains the same) ---
  const marketData = [
    { title: "Total Market Volume", value: "1.2T+", icon: DollarSign },
    { title: "Market Growth Rate", value: "+3.5%/week", icon: TrendingUp },
    { title: "Active Traders", value: "15K+", icon: PersonStanding },
  ];
  // ... (upcomingEvents, features, newsItems remain the same)
  const upcomingEvents = [
    { name: "Dwarven Mines Fiesta", date: "Next Week", expectedFlip: "+60% on Mithril/Titanium" },
    { name: "Spooky Festival", date: "In 2 Weeks", expectedFlip: "+120% on Candy/Spooky Items" },
    { name: "Jerry's Workshop Opening", date: "In 3 Weeks", expectedFlip: "+80% on Gifts/Winter Items" },
  ];

  const features = [
    { title: "Real-time Market Tracking", description: "Monitor changes in the market as they happen with our advanced real-time tracking system. Get instant updates on profitable oppertunites, allowing you to make quick decisions and capitalize on market inefficiencies.", icon: TrendingUp, },
    { title: "Comprehensive Analysis", description: "Gain deep insights into market trends with our powerful analytical tools. Analyze historical data, identify patterns, and understand market dynamics to make informed trading decisions.", icon: PieChart, },
    { title: "Formula-Driven Forecasts", description: "Utilize our rigorously tested formulas to decipher market trends. Our carefully engineered calculations analyze extensive data, delivering dependable price forecasts that empower you with a competitive advantage in the bazaar..", icon: BarChart2, },
    { title: "Risk Management Alerts", description: "Stay informed about potential market risks with our alert system. Receive clear, useful information about market volatility, price fluctuations, and other factors that could impact your trading strategy.", icon: AlertTriangle, },
    { title: "Customizable Strategies", description: "Implement and test automated trading strategies based on your custom parameters. Our platform allows you to micromanage your budget, time limitations and margins to maximize your profit potential.", icon: Zap, },
    { title: "Performance Benchmarking", description: "Compare your trading performance against top traders and market indices. Gain valuable insights into your strengths and areas for improvement, helping you refine your strategies and boost your profits.", icon: Target, },
  ];

  const newsItems = [
    { title: "Alpha v0.3 Deployed", excerpt: "New formula adjustments and UI tweaks for better performance.", date: "Oct 26, 2023", link: "#", },
    { title: "Community Discord Launched", excerpt: "Join our Discord to discuss strategies and report bugs!", date: "Oct 15, 2023", link: "#", },
    { title: "Wiz Goes Live!", excerpt: "Initial beta release of Wiz, the Bazaar Profit Wizard.", date: "Oct 01, 2023", link: "#", },
  ];

  // --- Data fetching logic ---
  type OptimizerDataType = KoyebOptimizerResponse | ApiErrorResponse | null;
  // We might not need failedItemsData directly on this page if BazaarProfitableItems handles it,
  // but fetching it here could be useful for a global status or if other components need it.
  // For now, let's assume BazaarProfitableItems primarily needs optimizerData.
  // type FailedItemsDataType = KoyebFailedItemsReportResponse | ApiErrorResponse | null;

  let optimizerData: OptimizerDataType = null;
  // let failedItemsData: FailedItemsDataType = null; // If needed
  let optimizerLoading = true;
  // let failedItemsLoading = true; // If needed

  const OPTIMIZER_DATA_URL = '/api/optimizer_results';
  // const FAILED_ITEMS_URL = '/api/failed_items_report'; // If needed

  async function loadData<T>(url: string): Promise<T | ApiErrorResponse> {
    try {
      const response = await fetch(url);
      const data = await response.json();
      if (!response.ok) {
        const message = (data as ApiErrorResponse)?.error || `HTTP error! Status: ${response.status}`;
        throw new Error(message);
      }
      return data as T;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown fetch error';
      console.error(`Failed to fetch from ${url}:`, errorMessage);
      return { error: errorMessage };
    }
  }

  onMount(async () => {
    optimizerData = await loadData<KoyebOptimizerResponse>(OPTIMIZER_DATA_URL);
    optimizerLoading = false;

    // If you also need failed items data on this page:
    // failedItemsData = await loadData<KoyebFailedItemsReportResponse>(FAILED_ITEMS_URL);
    // failedItemsLoading = false;
  });

  function isError(data: any): data is ApiErrorResponse {
    return data && typeof data.error === 'string';
  }
</script>

<!-- Fixed Header (remains the same) -->
<header class="fixed top-0 left-0 right-0 z-50 bg-darker bg-opacity-90 backdrop-blur-sm shadow-md">
  <div class="container mx-auto px-4 sm:px-6 lg:px-8 py-3 flex justify-between items-center">
    <a href="/" class="text-3xl font-bold text-accent lowercase hover:opacity-80 transition-opacity">wiz</a>
    <nav>
      <ul class="flex space-x-4 sm:space-x-6">
        <li><a href="/" class="text-light hover:text-accent transition-colors">Home</a></li>
        <li><a href="/bazaaritems" class="text-light hover:text-accent transition-colors">Items</a></li>
        <li><a href="#features" class="text-light hover:text-accent transition-colors">Features</a></li>
      </ul>
    </nav>
  </div>
</header>

<main class="pt-16 sm:pt-20">
  <!-- Hero Section (remains the same) -->
  <section class="pt-24 pb-20 sm:pt-32 sm:pb-24 px-4 bg-dark text-center">
    <div class="container mx-auto max-w-3xl">
      <h1 class="text-5xl sm:text-6xl font-bold mb-6 text-accent lowercase">wiz</h1>
      <p class="text-lg sm:text-xl mb-8 text-light opacity-80 leading-relaxed">
        With the power of advanced analytics and a bit of magic, wiz transforms complex trading data into strategies you can use to gain a competitive edge in the Skyblock market.
      </p>
      <a
        href="#bazaar-items-section" 
        class="inline-block px-8 py-3 sm:px-10 sm:py-4 bg-primary text-light rounded-md font-semibold hover:bg-secondary transition-colors text-lg"
      >
        View Profitable Items
      </a>
    </div>
  </section>

  <!-- Bazaar Items Section - MODIFIED to pass data -->
  <section id="bazaar-items-section" class="py-16 sm:py-24 px-4 bg-darker">
    <div class="container mx-auto max-w-7xl">
      <h2 class="text-3xl sm:text-4xl font-bold mb-10 sm:mb-12 text-accent text-center">Top Profitable Bazaar Flips</h2>
      {#if optimizerLoading}
        <p class="text-center text-light opacity-80">Loading profitable items...</p>
      {:else if optimizerData && !isError(optimizerData)}
        <BazaarProfitableItems bind:optimizerResults={optimizerData.results} bind:optimizerSummary={optimizerData.summary} />
        <!-- Or pass the whole optimizerData object if BazaarProfitableItems expects that -->
        <!-- <BazaarProfitableItems data={optimizerData} /> -->
      {:else if optimizerData && isError(optimizerData)}
        <p class="text-center text-red-500">Error loading profitable items: {optimizerData.error}</p>
      {:else}
        <p class="text-center text-light opacity-80">Could not load profitable items data.</p>
      {/if}
    </div>
  </section>

  <!-- Market Overview Section (remains the same) -->
  <section id="market-overview" class="py-16 sm:py-24 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl sm:text-4xl font-bold mb-10 sm:mb-12 text-accent">Market Overview</h2>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6 sm:gap-8 mb-12 sm:mb-16">
        {#each marketData as item}
          <div class="bg-darker bg-opacity-70 p-6 rounded-lg border border-secondary/30 shadow-lg hover:shadow-primary/30 transition-shadow">
            <svelte:component this={item.icon} class="w-10 h-10 mx-auto mb-4 text-accent" />
            <h3 class="text-lg font-semibold mb-2 text-light">{item.title}</h3>
            <p class="text-2xl font-bold text-primary">{item.value}</p>
          </div>
        {/each}
      </div>
      <div class="bg-darker bg-opacity-70 p-6 sm:p-8 rounded-lg border border-secondary/30 shadow-lg text-left max-w-3xl mx-auto hover:shadow-primary/30 transition-shadow">
        <h3 class="text-2xl font-bold mb-6 text-accent text-center">Upcoming Skyblock Events</h3>
        <ul class="space-y-4">
          {#each upcomingEvents as event}
            <li class="flex items-start space-x-4 p-3 bg-dark rounded-md hover:bg-dark/50 transition-colors">
              <Calendar class="w-6 h-6 text-accent flex-shrink-0 mt-1" />
              <div>
                <h4 class="text-lg font-semibold text-light">{event.name}</h4>
                <p class="text-sm text-light opacity-70">{event.date}</p>
                <p class="text-sm font-medium text-primary">{event.expectedFlip}</p>
              </div>
            </li>
          {/each}
        </ul>
      </div>
    </div>
  </section>

  <!-- Features Section (remains the same) -->
  <section id="features" class="py-16 sm:py-24 px-4 bg-darker">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl sm:text-4xl font-bold mb-10 sm:mb-12 text-accent">Powerful Features</h2>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8 sm:gap-12">
        {#each features as feature}
          <div class="bg-dark p-6 rounded-lg border border-secondary/30 shadow-lg hover:border-primary/50 transition-all duration-300 ease-in-out transform hover:scale-105">
            <svelte:component this={feature.icon} class="w-12 h-12 mx-auto mb-5 text-accent" />
            <h3 class="text-xl font-semibold mb-3 text-primary">{feature.title}</h3>
            <p class="text-light opacity-80 leading-relaxed text-sm">{feature.description}</p>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- News Section (remains the same) -->
  <section id="news" class="py-16 sm:py-24 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl sm:text-4xl font-bold mb-10 sm:mb-12 text-accent">Latest Updates</h2>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-8 sm:gap-12">
        {#each newsItems as item}
          <article class="bg-darker p-6 rounded-lg border border-secondary/30 shadow-lg text-left hover:shadow-primary/30 transition-shadow">
            <h3 class="text-xl font-semibold mb-3 text-primary">{item.title}</h3>
            <p class="text-light opacity-80 mb-4 leading-relaxed text-sm">{item.excerpt}</p>
            <div class="flex justify-between items-center mt-auto">
              <span class="text-xs text-light opacity-60">{item.date}</span>
              <a href={item.link} class="text-accent hover:opacity-80 transition-colors text-sm font-medium" target="_blank" rel="noopener noreferrer">
                Read More →
              </a>
            </div>
          </article>
        {/each}
      </div>
    </div>
  </section>

  <!-- Footer (remains the same) -->
  <footer class="py-12 sm:py-16 px-4 bg-darker border-t border-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-8 mb-8">
        <div>
          <h3 class="text-2xl font-bold text-accent mb-3 lowercase">wiz</h3>
          <p class="text-light opacity-60 leading-relaxed text-sm">
            Empowering bazaar traders with advanced analytics and magic.
          </p>
        </div>
        <div>
          <h4 class="text-lg font-semibold text-primary mb-3">Quick Links</h4>
          <ul class="space-y-1.5">
            <li><a href="/" class="text-light opacity-70 hover:opacity-100 hover:text-accent transition-colors text-sm">Home</a></li>
            <li><a href="#features" class="text-light opacity-70 hover:opacity-100 hover:text-accent transition-colors text-sm">Features</a></li>
            <li><a href="#news" class="text-light opacity-70 hover:opacity-100 hover:text-accent transition-colors text-sm">News</a></li>
            <li><a href="/bazaaritems" class="text-light opacity-70 hover:opacity-100 hover:text-accent transition-colors text-sm">Bazaar Items</a></li>
          </ul>
        </div>
        <div>
          <h4 class="text-lg font-semibold text-primary mb-3">Contact Us</h4>
          <p class="text-light opacity-70 mb-1 text-sm">Email: support@example-wiz.com</p>
          <p class="text-light opacity-70 text-sm">Discord: discord.gg/examplewiz</p>
        </div>
      </div>
      <div class="mt-8 pt-6 border-t border-dark/50">
        <p class="text-light opacity-50 text-xs">© {new Date().getFullYear()} wiz. All rights reserved. Not affiliated with Hypixel Inc.</p>
      </div>
    </div>
  </footer>
</main>

<style>
  html {
    scroll-behavior: smooth;
  }
</style>