<script lang="ts">
    import { onMount } from 'svelte';
    import type { AvgPriceData, PriceHistory } from '$lib/utils/typesAndTransforms'; // Import common types
    import { formatLargeNumberForChart as formatLargeNumber } from '$lib/utils/typesAndTransforms'; // Import specific formatter

    // --- Component Props ---
    export let avgData: AvgPriceData | null = null;
    export let width: number = 700;
    export let height: number = 300;
    export let padding: number = 70;
  
    // --- Historical Data Processing ---
    let averagePrices: { avg: number; timestamp: string }[] = [];
    $: averagePrices = avgData && avgData.history
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
    let auctionPriceLoading = true;
    onMount(async () => {
      if (!avgData || !avgData.item) { // Ensure avgData and avgData.item exist
          auctionPriceLoading = false;
          return;
      }
      try {
        const res = await fetch('/lowestbin.json'); // Make sure this file exists in /static
        if (!res.ok) {
          console.error('Failed to fetch lowestbin.json', res.status, await res.text().catch(()=>""));
          return;
        }
        const auctionData = await res.json();
        if (auctionData && typeof auctionData === 'object' && auctionData[avgData.item] !== undefined) {
          auctionPrice = auctionData[avgData.item];
        } else {
            // console.log(`No auction price found for ${avgData.item} in lowestbin.json`);
        }
      } catch (err) {
        console.error('Error fetching lowestbin.json:', err);
      } finally {
          auctionPriceLoading = false;
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
    $: proximityPercent = actualAveragePrice && currentPrice // Ensure currentPrice is also valid
      ? ((currentPrice - actualAveragePrice) / actualAveragePrice) * 100
      : 0;
  
    // --- Y-Axis Scaling (50% margin) ---
    let dataMin = 0, dataMax = 0;
    $: {
      if (sortedPrices.length > 0) {
        const prices = sortedPrices.map(p => p.avg);
        if (auctionPrice !== null) prices.push(auctionPrice); // Include auction price in range

        dataMin = Math.min(...prices);
        dataMax = Math.max(...prices);
      } else if (auctionPrice !== null) { // Only auction price
        dataMin = auctionPrice;
        dataMax = auctionPrice;
      } else { // No data at all
        dataMin = 0;
        dataMax = 0;
      }
    }

    let scaledMin = 0, scaledMax = 0;
    $: {
      if (dataMin === 0 && dataMax === 0 && auctionPrice === null && sortedPrices.length === 0) { // Truly no data
        scaledMin = 0;
        scaledMax = 100; // Default range if no data at all
      } else if (dataMin === dataMax) { // Single data point (or all points are the same)
        const val = dataMin; // or dataMax, they are the same
        scaledMin = val === 0 ? 0 : Math.max(0, Math.round((val * 0.8) * 10) / 10); // Ensure not negative, 20% margin
        scaledMax = val === 0 ? 100 : Math.round((val * 1.2) * 10) / 10;       // 20% margin
        if (scaledMin === scaledMax && scaledMin === 0) scaledMax = 100; // Handle 0 value case
        else if (scaledMin === scaledMax) scaledMax = scaledMin + (scaledMin * 0.2 || 10); // Add a bit if still same
      } else {
        const range = dataMax - dataMin;
        scaledMin = Math.max(0, Math.round((dataMin - 0.2 * range) * 10) / 10); // 20% margin, ensure not negative
        scaledMax = Math.round((dataMax + 0.2 * range) * 10) / 10;           // 20% margin
      }
       // Ensure scaledMax is always greater than scaledMin
      if (scaledMax <= scaledMin) {
        scaledMax = scaledMin + (scaledMin * 0.1 || 10); // Add a small amount if they are equal or max is less
      }
    }
  
    // --- Area Chart Coordinates (Bazaar Items) ---
    let xs: number[] = [];
    let ys: number[] = [];
    $: {
      if (sortedPrices.length === 0 || !avgData) { // Guard against no sortedPrices
          xs = []; ys = [];
      } else if (sortedPrices.length === 1) {
        xs = [padding, width - padding];
        const yVal =
          scaledMax === scaledMin
            ? height / 2
            : height - padding - ((sortedPrices[0].avg - scaledMin) / (scaledMax - scaledMin)) * (height - 2 * padding);
        ys = [yVal, yVal];
      } else {
        xs = sortedPrices.map((_, i) =>
          padding + i * ((width - 2 * padding) / (sortedPrices.length - 1))
        );
        ys = sortedPrices.map(p => {
          if (scaledMax === scaledMin) return height / 2; // Avoid division by zero
          return height - padding - ((p.avg - scaledMin) / (scaledMax - scaledMin)) * (height - 2 * padding);
        });
      }
    }
  
    let areaPath: string = "";
    let linePath: string = "";
    $: {
       if (xs.length === 0 || ys.length === 0) { // Guard against empty coords
          areaPath = ""; linePath = "";
      } else if (xs.length === 1 || (xs.length === 2 && sortedPrices.length === 1) ) { // Single point or effectively single point for line
        areaPath = `M ${xs[0]} ${ys[0]} L ${xs[xs.length-1]} ${ys[0]} L ${xs[xs.length-1]} ${height - padding} L ${xs[0]} ${height - padding} Z`;
        linePath = `M ${xs[0]} ${ys[0]} L ${xs[xs.length-1]} ${ys[0]}`;
      } else {
        let d = `M ${xs[0]} ${ys[0]}`;
        for (let i = 1; i < xs.length; i++) {
          const cpx1 = xs[i-1] + (xs[i] - xs[i-1]) / 3;
          const cpy1 = ys[i-1];
          const cpx2 = xs[i] - (xs[i] - xs[i-1]) / 3;
          const cpy2 = ys[i];
          d += ` C ${cpx1} ${cpy1}, ${cpx2} ${cpy2}, ${xs[i]} ${ys[i]}`;
        }
        linePath = d;
        areaPath = d + ` L ${xs[xs.length - 1]} ${height - padding} L ${xs[0]} ${height - padding} Z`;
      }
    }
  
    let xLabelStart: string = "";
    let xLabelEnd: string = "";
    $: {
      if (sortedPrices.length > 0) {
        xLabelStart = new Date(sortedPrices[0].timestamp).toLocaleDateString(undefined, {month: 'short', day: 'numeric'});
        xLabelEnd = new Date(sortedPrices[sortedPrices.length - 1].timestamp).toLocaleDateString(undefined, {month: 'short', day: 'numeric'});
      } else {
        xLabelStart = "Start"; xLabelEnd = "End";
      }
    }
  
    let auctionBar: { x: number; y: number; width: number; height: number } | null = null;
    $: if (auctionPrice !== null && auctionPriceLoading === false) { // Only calculate if auctionPrice is loaded
      const barValue = auctionPrice; // Use actual auction price for the bar
      const barWidth = Math.max(20, (width - 2 * padding) * 0.15); // 15% of chart width, min 20px
      const x = width / 2 - barWidth / 2; 
      
      let yPos = height - padding; // Default to bottom if out of scale
      let barHeightValue = 0;

      if (scaledMax > scaledMin) { // Ensure valid scale
          yPos = height - padding - ((barValue - scaledMin) / (scaledMax - scaledMin)) * (height - 2 * padding);
          // Clamp yPos within chart bounds
          yPos = Math.max(padding, Math.min(yPos, height - padding));
          barHeightValue = (height - padding) - yPos;
      } else if (barValue === scaledMin) { // If scale is flat and barValue matches
          yPos = height - padding;
          barHeightValue = 0; // or a minimal height
      }


      auctionBar = { x, y: yPos, width: barWidth, height: Math.max(0, barHeightValue) }; // Ensure height is not negative
    } else {
      auctionBar = null;
    }
  
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
        pointerX = auctionBar.x + auctionBar.width / 2; // Center of the bar
        pointerY = auctionBar.y; // Top of the bar
        pointerValue = auctionPrice;
        pointerTime = "Current BIN";
      } else if (sortedPrices.length > 0 && xs.length > 0 && ys.length > 0) {
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
        // Ensure index is within bounds
        if (closestIndex < ys.length && closestIndex < sortedPrices.length) {
            pointerY = ys[closestIndex];
            pointerValue = sortedPrices[closestIndex].avg;
            pointerTime = sortedPrices[closestIndex].timestamp;
        } else {
            // Fallback if something is off with indices
            pointerX = null; pointerY = null; pointerValue = null; pointerTime = null;
        }
      } else {
          pointerX = null; pointerY = null; pointerValue = null; pointerTime = null;
      }
    }
  
    function handlePointerDown(event: PointerEvent): void {
      if ((auctionPrice === null && sortedPrices.length === 0)) return; // No data to interact with
      isDragging = true;
      updatePointer(event);
    }
  
    function handlePointerMove(event: PointerEvent): void {
      if (isDragging) updatePointer(event);
    }
  
    function handlePointerUp(): void { // event param removed as not used
      isDragging = false;
      // Persist pointer info if not auction bar, or clear if preferred
      if (auctionPrice !== null && auctionBar) {
        // Keep auction pointer or clear, depends on desired UX
        // For now, let's clear it like the line chart
        pointerX = null; pointerY = null; pointerValue = null; pointerTime = null;
      } else if (!isDragging && sortedPrices.length > 0) { // Keep last point if not auction
        // No, let's clear on mouse up too for consistency
         pointerX = null; pointerY = null; pointerValue = null; pointerTime = null;
      }
    }
  
    function handlePointerLeave(): void {
      if (isDragging) isDragging = false; // Stop dragging if mouse leaves while pressed
      pointerX = null;
      pointerY = null;
      pointerValue = null;
      pointerTime = null;
    }
  </script>
  
  <div class="mx-auto p-1 md:p-4 rounded-lg bg-dark shadow-lg">
    {#if auctionPriceLoading && (!avgData || sortedPrices.length === 0)}
        <div style="height: {height}px;" class="flex items-center justify-center text-gray-400">
            Loading chart data...
        </div>
    {:else if (!avgData || (sortedPrices.length === 0 && auctionPrice === null))}
      <div style="height: {height}px;" class="flex items-center justify-center text-gray-400">
        No price data available for this item.
      </div>
    {:else}
      <svg
        {width}
        {height}
        class="mx-auto cursor-grab active:cursor-grabbing"
        on:pointerdown={handlePointerDown}
        on:pointermove={handlePointerMove}
        on:pointerup={handlePointerUp}
        on:pointerleave={handlePointerLeave}
        viewBox="0 0 {width} {height}"
        preserveAspectRatio="xMidYMid meet"
      >
        <defs>
            <linearGradient id="areaGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                <stop offset="0%" style="stop-color:rgba(104,177,122,0.4);stop-opacity:1" />
                <stop offset="100%" style="stop-color:rgba(104,177,122,0.05);stop-opacity:1" />
            </linearGradient>
             <filter id="shadow" x="-20%" y="-20%" width="140%" height="140%">
                <feDropShadow dx="0" dy="2" stdDeviation="2" flood-color="#000000" flood-opacity="0.2"/>
            </filter>
        </defs>

        <line x1={padding} y1={height - padding} x2={width - padding} y2={height - padding} stroke="#4A5568" stroke-width="1" />
        <line x1={padding} y1={padding} x2={padding} y2={height - padding} stroke="#4A5568" stroke-width="1" />
  
        {#if auctionPrice !== null && auctionBar}
          <rect
            x={auctionBar.x}
            y={auctionBar.y}
            width={auctionBar.width}
            height={auctionBar.height}
            fill="url(#areaGradient)"
            stroke="#68b17a"
            stroke-width="2"
            rx="3"
            ry="3"
            style="filter:url(#shadow);"
          />
        {:else if linePath && areaPath && sortedPrices.length > 0}
          <path d={areaPath} fill="url(#areaGradient)" stroke="none" />
          <path d={linePath} fill="none" stroke="#68b17a" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="filter:url(#shadow);" />
        {/if}
  
        {#if isDragging && pointerX !== null && pointerY !== null && pointerValue !== null}
          <line x1={pointerX} y1={padding - 5} x2={pointerX} y2={height - padding + 5} stroke="#C8ACD6" stroke-width="1" stroke-dasharray="3 3" />
          {#if !(auctionPrice !== null && auctionBar)} <!-- No dot on auction bar, bar itself is the indicator -->
            <circle cx={pointerX} cy={pointerY} r="4" fill="#68b17a" stroke="white" stroke-width="1.5" />
          {/if}
          <g transform="translate({pointerX}, {pointerY - 10})">
            <rect x="-35" y="-18" width="70" height="20" rx="3" ry="3" fill="rgba(42, 42, 60, 0.85)" stroke="rgba(104,177,122,0.7)" stroke-width="1"/>
            <text text-anchor="middle" fill="#E2E8F0" font-size="11" font-weight="semibold">
                {formatLargeNumber(pointerValue)}
            </text>
          </g>
        {/if}
  
        <text x={padding - 8} y={padding + 4} text-anchor="end" fill="#A0AEC0" font-size="10">{formatLargeNumber(scaledMax)}</text>
        <text x={padding - 8} y={height - padding -1} text-anchor="end" fill="#A0AEC0" font-size="10">{formatLargeNumber(scaledMin)}</text>
  
        {#if auctionPrice === null} <!-- Only show date range if not auction bar -->
            <text x={padding} y={height - padding + 15} text-anchor="start" fill="#A0AEC0" font-size="10">{xLabelStart}</text>
            <text x={width - padding} y={height - padding + 15} text-anchor="end" fill="#A0AEC0" font-size="10">{xLabelEnd}</text>
        {/if}
      </svg>
  
      <div class="mt-3 px-2 text-center text-xs md:text-sm text-gray-400 space-y-1">
        {#if auctionPrice !== null}
            <p>Current BIN: <strong class="text-accent">{formatLargeNumber(auctionPrice)}</strong></p>
        {:else if sortedPrices.length > 0}
            <p>Latest Price: <strong class="text-accent">{formatLargeNumber(currentPrice)}</strong> ({new Date(sortedPrices[sortedPrices.length-1].timestamp).toLocaleDateString(undefined, {month:'short', day:'numeric'})})</p>
        {/if}
        {#if sortedPrices.length > 0}
            <p>Historical Avg: <strong class="text-light">{formatLargeNumber(actualAveragePrice)}</strong> 
            {#if auctionPrice === null} <!-- Only show proximity if it's not auction only chart -->
                (<span class={proximityPercent > 0 ? 'text-green-400' : proximityPercent < 0 ? 'text-red-400' : 'text-gray-400'}>
                    {proximityPercent > 0 ? '+' : ''}{proximityPercent.toFixed(1)}%
                </span>)
            {/if}
            </p>
        {/if}
        {#if isDragging && pointerValue !== null && pointerTime !== null}
          <p class="pt-1 border-t border-gray-700 mt-1">
            Selected: <strong class="text-accent">{formatLargeNumber(pointerValue)}</strong> 
            {#if pointerTime !== "Current BIN"}
                ({new Date(pointerTime).toLocaleDateString(undefined, {month:'short', day:'numeric', year: '2-digit'})})
            {:else}
                ({pointerTime})
            {/if}
          </p>
        {/if}
      </div>
    {/if}
  </div>