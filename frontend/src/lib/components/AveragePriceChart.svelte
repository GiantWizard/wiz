<script lang="ts">
    import { onMount } from 'svelte';
  
    // --- Type Definitions ---
    interface PriceHistory {
      buy: number;
      sell: number;
      timestamp: string;
    }
    interface AvgPriceData {
      item: string;
      history: PriceHistory[];
    }
  
    // --- Component Props ---
    export let avgData: AvgPriceData | null = null;
    export let width: number = 700;
    export let height: number = 300;
    export let padding: number = 70;
  
    // --- Updated Rounding Function with k/m/b suffix ---
    function formatLargeNumber(num: number): string {
      const abs = Math.abs(num);
      if (abs < 1000) {
        return (Math.round(num * 10) / 10).toString();
      } else if (abs < 1_000_000) {
        return (Math.round((num / 1000) * 10) / 10).toString() + 'k';
      } else if (abs < 1_000_000_000) {
        return (Math.round((num / 1_000_000) * 10) / 10).toString() + 'm';
      } else {
        return (Math.round((num / 1_000_000_000) * 10) / 10).toString() + 'b';
      }
    }
  
    // --- Historical Data Processing ---
    let averagePrices: { avg: number; timestamp: string }[] = [];
    $: averagePrices = avgData
      ? avgData.history.map(point => ({
          avg: (point.buy + point.sell) / 2,
          timestamp: point.timestamp
        }))
      : [];
  
    let sortedPrices: { avg: number; timestamp: string }[] = [];
    $: sortedPrices = [...averagePrices].sort(
      (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
    );
  
    let actualAveragePrice: number = 0;
    $: actualAveragePrice = sortedPrices.length
      ? sortedPrices.reduce((sum, p) => sum + p.avg, 0) / sortedPrices.length
      : 0;
  
    // --- Auction Price Fetch ---
    let auctionPrice: number | null = null;
    onMount(async () => {
      if (!avgData) return;
      try {
        const res = await fetch('/lowestbin.json');
        if (!res.ok) {
          console.error('Failed to fetch lowestbin.json');
          return;
        }
        const auctionData = await res.json();
        if (auctionData[avgData.item] !== undefined) {
          auctionPrice = auctionData[avgData.item];
        }
      } catch (err) {
        console.error('Error fetching lowestbin.json:', err);
      }
    });
  
    // --- Current Price & Proximity ---
    let currentPrice: number = 0;
    $: currentPrice =
      auctionPrice !== null
        ? auctionPrice
        : sortedPrices.length
        ? sortedPrices[sortedPrices.length - 1].avg
        : 0;
  
    let proximityPercent: number = 0;
    $: proximityPercent = actualAveragePrice
      ? ((currentPrice - actualAveragePrice) / actualAveragePrice) * 100
      : 0;
  
    // --- Y-Axis Scaling (50% margin) ---
    let dataMin = 0, dataMax = 0;
    $: {
      if (sortedPrices.length > 0) {
        dataMin = Math.min(...sortedPrices.map(p => p.avg));
        dataMax = Math.max(...sortedPrices.map(p => p.avg));
      }
    }
    let scaledMin = 0, scaledMax = 0;
    $: {
      if (sortedPrices.length === 0) {
        scaledMin = 0;
        scaledMax = 0;
      } else if (sortedPrices.length === 1) {
        const val = sortedPrices[0].avg;
        scaledMin = Math.round((val * 0.5) * 10) / 10;
        scaledMax = Math.round((val * 1.5) * 10) / 10;
      } else {
        const range = dataMax - dataMin;
        scaledMin = Math.round((dataMin - 0.5 * range) * 10) / 10;
        scaledMax = Math.round((dataMax + 0.5 * range) * 10) / 10;
      }
    }
  
    // --- Area Chart Coordinates (Bazaar Items) ---
    let xs: number[] = [];
    let ys: number[] = [];
    $: {
      if (sortedPrices.length === 1) {
        xs = [padding, width - padding];
        const yVal =
          scaledMax === scaledMin
            ? height / 2
            : height - padding - ((sortedPrices[0].avg - scaledMin) / (scaledMax - scaledMin)) * (height - 2 * padding);
        ys = [yVal, yVal];
      } else {
        xs = sortedPrices.map((_, i) =>
          i * ((width - 2 * padding) / (sortedPrices.length - 1)) + padding
        );
        ys = sortedPrices.map(p => {
          if (scaledMax === scaledMin) return height / 2;
          return height - padding - ((p.avg - scaledMin) / (scaledMax - scaledMin)) * (height - 2 * padding);
        });
      }
    }
  
    // --- Build Smooth SVG Paths for Area Chart using Bézier Curves ---
    let areaPath: string = "";
    let linePath: string = "";
    $: {
      if (sortedPrices.length === 1) {
        areaPath = `M ${padding} ${ys[0]} L ${width - padding} ${ys[0]} L ${width - padding} ${height - padding} L ${padding} ${height - padding} Z`;
        linePath = `M ${padding} ${ys[0]} L ${width - padding} ${ys[0]}`;
      } else {
        let d = `M ${xs[0]} ${ys[0]}`;
        for (let i = 1; i < xs.length; i++) {
          const midX = (xs[i - 1] + xs[i]) / 2;
          d += ` C ${midX} ${ys[i - 1]}, ${midX} ${ys[i]}, ${xs[i]} ${ys[i]}`;
        }
        linePath = d;
        areaPath = d + ` L ${xs[xs.length - 1]} ${height - padding} L ${xs[0]} ${height - padding} Z`;
      }
    }
  
    // --- X-Axis Labels ---
    let xLabelStart: string = "";
    let xLabelEnd: string = "";
    $: {
      if (sortedPrices.length > 0) {
        xLabelStart = new Date(sortedPrices[0].timestamp).toLocaleDateString();
        xLabelEnd = new Date(sortedPrices[sortedPrices.length - 1].timestamp).toLocaleDateString();
      }
    }
  
    // --- Auction Bar Chart for Auction Items ---
    // Use the average price (not the current price) for the auction bar.
    let auctionBar: { x: number; y: number; width: number; height: number } | null = null;
    $: if (auctionPrice !== null) {
      const barValue = actualAveragePrice;
      const barWidth = 80; // fixed width for auction bar
      const x = width / 2 - barWidth / 2; // center horizontally
      const y = height - padding - ((barValue - scaledMin) / (scaledMax - scaledMin)) * (height - 2 * padding);
      const barHeight = (height - padding) - y;
      auctionBar = { x, y, width: barWidth, height: barHeight };
    } else {
      auctionBar = null;
    }
  
    // --- Interactive Pointer ---
    let isDragging = false;
    let pointerX: number | null = null;
    let pointerY: number | null = null;
    let pointerValue: number | null = null;
    let pointerTime: string | null = null;
  
    function updatePointer(event: PointerEvent): void {
      const svg = event.currentTarget as SVGSVGElement;
      const rect = svg.getBoundingClientRect();
      const x = event.clientX - rect.left;
      if (auctionPrice !== null && auctionBar) {
        // Clamp pointerX within the auction bar.
        pointerX = Math.max(auctionBar.x, Math.min(x, auctionBar.x + auctionBar.width));
        pointerY = auctionBar.y;
        pointerValue = actualAveragePrice;
        pointerTime = sortedPrices.length ? sortedPrices[sortedPrices.length - 1].timestamp : "";
      } else {
        pointerX = Math.max(padding, Math.min(x, width - padding));
        let closestIndex = 0;
        let minDiff = Infinity;
        xs.forEach((xVal, i) => {
          const diff = Math.abs(xVal - pointerX!);
          if (diff < minDiff) {
            minDiff = diff;
            closestIndex = i;
          }
        });
        pointerY = ys[closestIndex];
        pointerValue = sortedPrices[closestIndex].avg;
        pointerTime = sortedPrices[closestIndex].timestamp;
      }
    }
  
    // --- Modified Pointer Event Handlers ---
    function handlePointerDown(event: PointerEvent): void {
      isDragging = true;
      updatePointer(event);
    }
  
    function handlePointerMove(event: PointerEvent): void {
      if (isDragging) updatePointer(event);
    }
  
    function handlePointerUp(event: PointerEvent): void {
      isDragging = false;
      pointerX = null;
      pointerY = null;
      pointerValue = null;
      pointerTime = null;
    }
  
    function handlePointerLeave(): void {
      pointerX = null;
      pointerY = null;
      pointerValue = null;
      pointerTime = null;
    }
  </script>
  
  <div class="mx-auto p-4">
    {#if avgData && sortedPrices.length > 0}
      <svg
        {width}
        {height}
        class="mx-auto cursor-pointer"
        on:pointerdown={handlePointerDown}
        on:pointermove={handlePointerMove}
        on:pointerup={handlePointerUp}
        on:pointerleave={handlePointerLeave}
      >
        <!-- X-axis -->
        <line
          x1={padding}
          y1={height - padding}
          x2={width - padding}
          y2={height - padding}
          stroke="#C8ACD6"
          stroke-width="1"
        />
        <!-- Y-axis -->
        <line
          x1={padding}
          y1={padding}
          x2={padding}
          y2={height - padding}
          stroke="#C8ACD6"
          stroke-width="1"
        />
  
        {#if auctionPrice !== null && auctionBar}
          <!-- Auction Items: Bar Chart -->
          <rect
            x={auctionBar.x}
            y={auctionBar.y}
            width={auctionBar.width}
            height={auctionBar.height}
            fill="rgba(67,61,139,0.3)"
            stroke="none"
            rx="4"
            ry="4"
          />
          <!-- Borders on left, top, and right (4px) -->
          <line x1={auctionBar.x} y1={auctionBar.y} x2={auctionBar.x} y2={auctionBar.y + auctionBar.height} stroke="#433D8B" stroke-width="4" />
          <line x1={auctionBar.x} y1={auctionBar.y} x2={auctionBar.x + auctionBar.width} y2={auctionBar.y} stroke="#433D8B" stroke-width="4" />
          <line x1={auctionBar.x + auctionBar.width} y1={auctionBar.y} x2={auctionBar.x + auctionBar.width} y2={auctionBar.y + auctionBar.height} stroke="#433D8B" stroke-width="4" />
        {:else}
          <!-- Bazaar Items: Smooth Area Chart -->
          <path
            d={areaPath}
            fill="rgba(67,61,139,0.3)"
            stroke="none"
          />
          <path
            d={linePath}
            fill="none"
            stroke="#433D8B"
            stroke-width="4"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        {/if}
  
        <!-- Interactive Pointer (only shown during click-drag) -->
        {#if pointerX !== null && pointerY !== null && pointerValue !== null}
        {#if isDragging}
        <!-- Vertical pointer line only during drag -->
        <line
            x1={pointerX}
            y1={padding}
            x2={pointerX}
            y2={height - padding}
            stroke="#68b17a"
            stroke-width="2"
            stroke-dasharray="4 2"
        />
        <!-- The pointer dot -->
        <circle
            cx={pointerX}
            cy={pointerY}
            r="5"
            fill="#68b17a"
            stroke="white"
            stroke-width="2"
        />
        <!-- Tooltip text displayed above the pointer dot -->
        <text
            x={pointerX}
            y={pointerY - 10}
            text-anchor="middle"
            fill="#68b17a"
            font-size="12"
        >
            {formatLargeNumber(pointerValue)}
        </text>
        {/if}
        {/if}

  
        <!-- Y-axis Labels -->
        <text
          x={padding - 10}
          y={padding}
          text-anchor="end"
          fill="#C8ACD6"
          font-size="12"
        >
          {formatLargeNumber(scaledMax)}
        </text>
        <text
          x={padding - 10}
          y={height - padding}
          text-anchor="end"
          fill="#C8ACD6"
          font-size="12"
        >
          {formatLargeNumber(scaledMin)}
        </text>
  
        <!-- X-axis Labels -->
        <text
          x={padding}
          y={height - padding + 20}
          text-anchor="middle"
          fill="#C8ACD6"
          font-size="12"
        >
          {xLabelStart}
        </text>
        <text
          x={width - padding}
          y={height - padding + 20}
          text-anchor="middle"
          fill="#C8ACD6"
          font-size="12"
        >
          {xLabelEnd}
        </text>
      </svg>
  
      <!-- Additional Info -->
      <div class="mt-4 text-center text-light">
        <p>
          Current Price: <strong>{formatLargeNumber(currentPrice)}</strong>
        </p>
        <p>
          Average Price: <strong>{formatLargeNumber(actualAveragePrice)}</strong>
        </p>
        <p>
          Proximity to Average: <strong>{Math.round(proximityPercent)}%</strong>
        </p>
        {#if pointerX !== null && pointerValue !== null && pointerTime !== null}
          <p>
            At {new Date(pointerTime).toLocaleDateString()}:
            <strong>{formatLargeNumber(pointerValue)}</strong>
          </p>
        {/if}
      </div>
    {:else}
      <p class="text-light">No average price data available.</p>
    {/if}
  </div>
  
  <style>
    /* Adjust styling as needed */
  </style>
  