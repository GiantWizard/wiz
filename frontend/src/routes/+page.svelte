<script>
  import { onMount } from 'svelte';
  // Import icons from lucide-svelte
  import { DollarSign, TrendingUp, Calendar, PieChart, BarChart2, AlertTriangle, Zap, Target } from 'lucide-svelte';

  // Market Overview Data
  const marketData = [
    { title: "Total Market Volume", value: "$1,250,000,000", icon: DollarSign },
    { title: "Market Growth Rate", value: "15% Monthly", icon: TrendingUp },
    { title: "Active Traders", value: "50,000+", icon: DollarSign },
  ];

  const upcomingEvents = [
    { name: "Summer Festival", date: "June 15-20", expectedFlip: "+200% on seasonal items" },
    { name: "New Dungeon Release", date: "July 5", expectedFlip: "+150% on high-tier weapons" },
    { name: "Double XP Weekend", date: "August 1-2", expectedFlip: "+100% on XP boosters" },
  ];

  // Features Data
  const features = [
    {
      title: "Real-time Profit Tracking",
      description:
        "Monitor your earnings as they happen with our advanced real-time tracking system. Get instant updates on your profits, allowing you to make quick decisions and capitalize on market opportunities.",
      icon: TrendingUp,
    },
    {
      title: "Comprehensive Market Analysis",
      description:
        "Gain deep insights into market trends with our powerful analytical tools. Analyze historical data, identify patterns, and understand market dynamics to make informed trading decisions.",
      icon: PieChart,
    },
    {
      title: "AI-Powered Price Predictions",
      description:
        "Leverage the power of artificial intelligence to forecast market trends. Our advanced AI algorithms analyze vast amounts of data to provide accurate price predictions, giving you a competitive edge in the bazaar.",
      icon: BarChart2,
    },
    {
      title: "Risk Management Alerts",
      description:
        "Stay informed about potential market risks with our intelligent alert system. Receive timely notifications about market volatility, price fluctuations, and other factors that could impact your trading strategy.",
      icon: AlertTriangle,
    },
    {
      title: "Automated Trading Strategies",
      description:
        "Implement and test automated trading strategies based on your custom parameters. Our platform allows you to create, backtest, and deploy trading bots that can execute trades 24/7, maximizing your profit potential.",
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
      title: "New Trading Algorithm Boosts Profits by 30%",
      excerpt:
        "Our latest AI-powered trading algorithm has shown remarkable results in recent tests, increasing profits by an average of 30% for beta testers.",
      date: "May 15, 2023",
      link: "/news/new-trading-algorithm",
    },
    {
      title: "Upcoming Bazaar Economy Changes: What You Need to Know",
      excerpt:
        "Major changes are coming to the bazaar economy next month. Learn how these updates will affect your trading strategies and how to prepare.",
      date: "May 10, 2023",
      link: "/news/bazaar-economy-changes",
    },
    {
      title: "Top 5 Most Profitable Items This Season",
      excerpt:
        "We've analyzed market data to identify the top 5 most profitable items for trading this season. Find out which items you should be focusing on.",
      date: "May 5, 2023",
      link: "/news/top-profitable-items",
    },
  ];
</script>

<!-- Fixed Header -->
<header class="fixed top-0 left-0 right-0 z-50 bg-darker bg-opacity-90 backdrop-blur-sm">
  <div class="container mx-auto px-4 py-4 flex justify-between items-center">
    <!-- Changed to lowercase to match the hero section -->
    <div class="text-2xl font-bold text-accent lowercase">wiz</div>
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
  <section class="pt-32 pb-24 px-4 bg-dark">
    <div class="container mx-auto max-w-4xl text-center">
      <h1 class="text-6xl font-bold mb-6 text-accent lowercase">wiz</h1>
      <p class="text-xl mb-8 text-light opacity-80">
        Harness the power of advanced analytics and AI-driven insights to dominate the bazaar market. Wiz transforms
        complex trading data into actionable strategies, giving you the competitive edge in virtual economies.
      </p>
      <a href="/get-started" class="inline-block px-8 py-4 bg-primary text-light rounded-md font-semibold hover:bg-secondary transition-colors">
        Start Maximizing Profits
      </a>
    </div>
  </section>

  <!-- Market Overview Section -->
  <section class="py-24 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl font-bold mb-12 text-accent">Market Overview</h2>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-8 mb-16">
        {#each marketData as item}
          <div class="bg-dark bg-opacity-70 p-6 rounded-lg border border-secondary/50 shadow-md">
            <svelte:component this={item.icon} class="w-8 h-8 text-darker mb-4" />
            <h3 class="text-lg font-semibold mb-2 text-light">{item.title}</h3>
            <p class="text-2xl font-bold text-accent">{item.value}</p>
          </div>
        {/each}
      </div>
      <div class="bg-dark bg-opacity-70 p-6 rounded-lg border border-secondary/50 shadow-md">
        <h3 class="text-2xl font-bold mb-6 text-accent">Upcoming Events & Flip Opportunities</h3>
        <ul class="space-y-4">
          {#each upcomingEvents as event}
            <li class="flex items-start space-x-4">
              <Calendar class="w-6 h-6 text-darker flex-shrink-0 mt-1" />
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
            <svelte:component this={feature.icon} class="w-10 h-10 text-darker mb-4" />
            <h3 class="text-xl font-semibold mb-4 text-accent">{feature.title}</h3>
            <p class="text-light opacity-80">{feature.description}</p>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- News Section -->
  <section class="py-24 px-4 bg-dark">
    <div class="container mx-auto max-w-6xl text-center">
      <h2 class="text-3xl font-bold mb-12 text-accent">Latest News</h2>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-12">
        {#each newsItems as item}
          <div class="bg-dark bg-opacity-70 p-6 rounded-lg border border-secondary/50 shadow-md">
            <h3 class="text-xl font-semibold mb-4 text-accent">{item.title}</h3>
            <p class="text-light opacity-80 mb-4">{item.excerpt}</p>
            <div class="flex justify-between items-center">
              <span class="text-sm text-light opacity-60">{item.date}</span>
              <a href={item.link} class="text-darker hover:text-accent transition-colors">
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
        <div>
          <h3 class="text-xl font-bold text-accent mb-4 lowercase">wiz</h3>
          <p class="text-light opacity-60">
            Empowering bazaar traders with advanced analytics and AI-driven insights.
          </p>
        </div>
        <div>
          <h4 class="text-lg font-semibold text-accent mb-4">Quick Links</h4>
          <ul class="space-y-2">
            <li><a href="/" class="text-light opacity-60 hover:opacity-100">Home</a></li>
            <li><a href="/features" class="text-light opacity-60 hover:opacity-100">Features</a></li>
            <li><a href="/news" class="text-light opacity-60 hover:opacity-100">News</a></li>
            <li><a href="/pricing" class="text-light opacity-60 hover:opacity-100">Pricing</a></li>
          </ul>
        </div>
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
