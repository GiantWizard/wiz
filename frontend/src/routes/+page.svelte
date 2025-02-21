<script>
  import { onMount } from 'svelte';
  // Import icons from lucide-svelte
  import {
    DollarSign,
    TrendingUp,
    Calendar,
    PieChart,
    BarChart2,
    AlertTriangle,
    Zap,
    Target,
    PersonStanding

  } from 'lucide-svelte';

  // Market Overview Data
  const marketData = [
    { title: "Total Market Volume", value: "Large number here", icon: DollarSign },
    { title: "Market Growth Rate", value: "Large percent here", icon: TrendingUp },
    { title: "Active Traders", value: "Some amount of people", icon: PersonStanding },
  ];

  const upcomingEvents = [
    { name: "Cool Event", date: "Some days ago", expectedFlip: "+100% on event items" },
    { name: "Some Festival", date: "More days ago", expectedFlip: "+100% on festivities" },
    { name: "Mayor election?!", date: "Even more days ago", expectedFlip: "+100% on mayor items" },
  ];

  // Features Data
  const features = [
    {
      title: "Real-time Market Tracking",
      description:
        "Monitor changes in the market as they happen with our advanced real-time tracking system. Get instant updates on profitable oppertunites, allowing you to make quick decisions and capitalize on market inefficiencies.",
      icon: TrendingUp,
    },
    {
      title: "Comprehensive Analysis",
      description:
        "Gain deep insights into market trends with our powerful analytical tools. Analyze historical data, identify patterns, and understand market dynamics to make informed trading decisions.",
      icon: PieChart,
    },
    {
      title: "Formula-Driven Forecasts",
      description:
        "Utilize our rigorously tested formulas to decipher market trends. Our carefully engineered calculations analyze extensive data, delivering dependable price forecasts that empower you with a competitive advantage in the bazaar..",
      icon: BarChart2,
    },
    {
      title: "Risk Management Alerts",
      description:
        "Stay informed about potential market risks with our alert system. Receive clear, useful information about market volatility, price fluctuations, and other factors that could impact your trading strategy.",
      icon: AlertTriangle,
    },
    {
      title: "Customizable Strategies",
      description:
        "Implement and test automated trading strategies based on your custom parameters. Our platform allows you to micromanage your budget, time limitations and margins to maximize your profit potential.",
      icon: Zap,
    },
    {
      title: "Performance Benchmarking",
      description:
        "Compare your trading performance against top traders and market indices. Gain valuable insights into your strengths and areas for improvement, helping you refine your strategies and boost your profits.",
      icon: Target,
    },
  ];

  // News Data
  const newsItems = [
    {
      title: "Last Update",
      excerpt:
        "Title of update",
        date: "Arbitrary date",
        link: "https://hypixel.net",
    },
    {
      title: "Last Update",
      excerpt:
        "Title of update",
      date: "Arbitrary date",
        link: "https://hypixel.net",
    },
    {
      title: "Last Update",
      excerpt:
        "Title of update",
      date: "Arbitrary date",
      link: "https://hypixel.net",
    },
  ];
</script>

<!-- Fixed Header -->
<header class="fixed top-0 left-0 right-0 z-50 bg-darker bg-opacity-90 backdrop-blur-sm">
  <div class="container mx-auto px-4 py-4 flex justify-between items-center">
    <!-- Brand -->
    <div class="text-2xl font-bold text-accent lowercase">wiz</div>

    <!-- Nav -->
    <nav>
      <ul class="flex space-x-6">
        <li>
          <a href="/" class="text-light hover:text-accent transition-colors">Home</a>
        </li>
        <li>
          <a href="/analytics" class="text-light hover:text-accent transition-colors">Analytics</a>
        </li>
        <li>
          <a href="/features" class="text-light hover:text-accent transition-colors">Features</a>
        </li>
      </ul>
    </nav>
  </div>
</header>

<main class="pt-20">
  <!-- Hero Section -->
  <section class="pt-32 pb-24 px-4 bg-dark text-center">
    <div class="container mx-auto max-w-4xl">
      <h1 class="text-6xl font-bold mb-6 text-accent lowercase">wiz</h1>
      <p class="text-xl mb-8 text-light opacity-80 leading-relaxed">
        With the power of advanced analytics and a bit of magic, wiz transforms complex trading data into strategies you can use to gain a competetive edge in the Skyblock market.
      </p>
      <a
        href="/get-started"
        class="inline-block px-8 py-4 bg-primary text-light rounded-md font-semibold hover:bg-secondary transition-colors"
      >
        Start Maximizing Profits
      </a>
    </div>
  </section>

  <!-- Market Overview Section -->
  <section class="py-24 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl font-bold mb-12 text-accent">Market Overview</h2>

      <!-- Market Stats -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-8 mb-16">
        {#each marketData as item}
          <div class="bg-dark bg-opacity-70 p-6 rounded-lg border border-secondary/50 shadow-md">
            <!-- Use a more visible color for icons -->
            <svelte:component
              this={item.icon}
              class="w-10 h-10 mx-auto mb-4 text-accent"
            />
            <h3 class="text-lg font-semibold mb-2 text-light">{item.title}</h3>
            <p class="text-2xl font-bold text-accent">{item.value}</p>
          </div>
        {/each}
      </div>

      <!-- Upcoming Events -->
      <div class="bg-dark bg-opacity-70 p-6 rounded-lg border border-secondary/50 shadow-md text-left max-w-3xl mx-auto">
        <h3 class="text-2xl font-bold mb-6 text-accent text-center">Upcoming Events</h3>
        <ul class="space-y-4">
          {#each upcomingEvents as event}
            <li class="flex items-start space-x-4">
              <!-- Make the icon more visible -->
              <Calendar class="w-6 h-6 text-accent flex-shrink-0 mt-1" />
              <div>
                <h4 class="text-lg font-semibold text-light">{event.name}</h4>
                <p class="text-sm text-light opacity-80">{event.date}</p>
                <p class="text-sm font-medium text-accent">{event.expectedFlip}</p>
              </div>
            </li>
          {/each}
        </ul>
      </div>
    </div>
  </section>

  <!-- Features Section -->
  <section class="py-24 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl font-bold mb-12 text-accent">Powerful Features</h2>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-12">
        {#each features as feature}
          <div class="bg-dark bg-opacity-70 p-6 rounded-lg border border-secondary/50 shadow-md">
            <svelte:component
              this={feature.icon}
              class="w-10 h-10 mx-auto mb-4 text-accent"
            />
            <h3 class="text-xl font-semibold mb-4 text-accent">{feature.title}</h3>
            <p class="text-light opacity-80 leading-relaxed">{feature.description}</p>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- News Section -->
  <section class="py-24 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl font-bold mb-12 text-accent">Latest Updates</h2>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-12">
        {#each newsItems as item}
          <div class="bg-dark bg-opacity-70 p-6 rounded-lg border border-secondary/50 shadow-md text-left">
            <h3 class="text-xl font-semibold mb-4 text-accent">{item.title}</h3>
            <p class="text-light opacity-80 mb-4 leading-relaxed">{item.excerpt}</p>
            <div class="flex justify-between items-center">
              <span class="text-sm text-light opacity-60">{item.date}</span>
              <a href={item.link} class="text-accent hover:opacity-80 transition-colors">
                Read More
              </a>
            </div>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- Footer Section -->
  <footer class="py-12 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
        <!-- Brand / About -->
        <div>
          <h3 class="text-xl font-bold text-accent mb-4 lowercase">wiz</h3>
          <p class="text-light opacity-60 leading-relaxed">
            Empowering bazaar traders with advanced analytics and magic.
          </p>
        </div>

        <!-- Quick Links -->
        <div>
          <h4 class="text-lg font-semibold text-accent mb-4">Quick Links</h4>
          <ul class="space-y-2">
            <li><a href="/" class="text-light opacity-60 hover:opacity-100">Home</a></li>
            <li><a href="/features" class="text-light opacity-60 hover:opacity-100">Features</a></li>
            <li><a href="/news" class="text-light opacity-60 hover:opacity-100">News</a></li>
            <li><a href="/pricing" class="text-light opacity-60 hover:opacity-100">Pricing</a></li>
          </ul>
        </div>

        <!-- Contact -->
        <div>
          <h4 class="text-lg font-semibold text-accent mb-4">Contact Us</h4>
          <p class="text-light opacity-60 mb-2">Email: support@wiz.com</p>
          <p class="text-light opacity-60">Phone: (123) 456-7890</p>
        </div>
      </div>

      <div class="mt-8 pt-8 border-t border-secondary/50">
        <p class="text-light opacity-60">&copy; {new Date().getFullYear()} wiz. All rights reserved.</p>
      </div>
    </div>
  </footer>
</main>

<style>
  /* 
    Tailwind handles most of your layout and color styling. 
    Add small overrides here if desired (e.g., custom transitions, 
    fine-tuned spacing, etc.). 
  */
</style>
