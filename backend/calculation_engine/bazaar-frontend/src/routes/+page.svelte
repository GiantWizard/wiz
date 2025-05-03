<script lang="ts">
	// Explicitly import onMount only if you uncomment the onMount call later
	// import { onMount } from 'svelte';

	// --- State Variables ---
	let itemId: string = 'PRECURSOR_GEAR'; // Default example item ID
	let quantity: number = 1; // Default quantity
	let isLoading: boolean = false; // Flag for loading state
	let errorMsg: string | null = null; // Store error message, initially null
	let apiResult: any = null; // Store the successful API response, initially null

	// --- API Fetch Function ---
	async function fetchCalculationData() {
		isLoading = true;
		errorMsg = null; // Clear previous error
		apiResult = null; // Clear previous result
		const backendUrl = `http://localhost:8080/calculate?id=${encodeURIComponent(
			itemId
		)}&qty=${encodeURIComponent(quantity)}`;

		console.log(`Fetching from backend: ${backendUrl}`);

		try {
			const response = await fetch(backendUrl);

			// Handle non-OK HTTP responses (like 400, 404, 500)
			if (!response.ok) {
				let errorJson = null;
				try {
					// Try to parse specific error from backend JSON response
					errorJson = await response.json();
				} catch (e) {
					// Ignore if response wasn't JSON
				}
				// Use backend error message if available, otherwise use HTTP status
				throw new Error(errorJson?.error || `HTTP error! Status: ${response.status}`);
			}

			// Parse the successful JSON response
			apiResult = await response.json();
			console.log('Backend API Result:', apiResult);

		} catch (e: any) {
			// Catch fetch errors (network issues) or errors thrown above
			console.error('Data fetch error:', e);
			errorMsg = e.message || 'An unknown error occurred while fetching data.';
		} finally {
			// Ensure loading state is always turned off
			isLoading = false;
		}
	}

	// Function to handle Enter key press in input fields
	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			fetchCalculationData();
		}
	}

	// --- Optional: Fetch on initial load ---
	// If you want to fetch data automatically when the page loads, uncomment this:
	// onMount(() => {
	//     console.log("Component mounted, fetching initial data...");
	//     fetchCalculationData();
	// });

</script>

<!-- This content is placed within the <slot /> of +layout.svelte -->
<div class="calculator-content">
	<h2>Bazaar Item Calculator</h2>

	<!-- Input Section -->
	<div class="input-group card">
		<label for="itemIdInput">Item ID:</label>
		<input
			id="itemIdInput"
			type="text"
			bind:value={itemId}
			on:keydown={handleKeydown}
			disabled={isLoading}
			placeholder="e.g., ENCHANTED_LAPIS_BLOCK"
		/>

		<label for="quantityInput">Quantity:</label>
		<input
			id="quantityInput"
			type="number"
			bind:value={quantity}
			min="1"
			on:keydown={handleKeydown}
			disabled={isLoading}
			placeholder="e.g., 1"
		/>

		<button on:click={fetchCalculationData} disabled={isLoading}>
			{#if isLoading}Calculating...{:else}Calculate{/if}
		</button>
	</div>

	<!-- Status/Results Section -->
	<div class="status-results">
		{#if isLoading}
			<p class="status">🔄 Loading data from backend...</p>
		{:else if errorMsg}
			<p class="status error">❌ Error: {errorMsg}</p>
		{:else if apiResult}
			<!-- Display results only if apiResult is not null -->
			<div class="results">
				<h3>
					Results for {apiResult.quantity} x {apiResult.normalizedProductId}
					{#if apiResult.normalizedProductId !== apiResult.inputProductId}
						<span class="subtle">(Input: {apiResult.inputProductId})</span>
					{/if}
				</h3>

				{#if apiResult.error}
					<p class="warning">⚠️ Calculation Warning/Error: {apiResult.error}</p>
				{/if}

				<!-- Direct Cost Card -->
				<section class="card">
					<h4>Direct Purchase Cost</h4>
					<p>
						Best C10M:
						<strong>{apiResult.directCost?.cost?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) ?? 'N/A'}</strong>
						({apiResult.directCost?.method ?? 'N/A'})
					</p>
					<p>Refill Rate (RR): {apiResult.directCost?.rr?.toFixed(2) ?? 'N/A'}</p>
					{#if apiResult.directCost?.error}
						<p class="error-detail">Error: {apiResult.directCost.error}</p>
					{/if}
				</section>

				<!-- Crafting Cost & Time Card -->
				<section class="card">
					<h4>Crafting Cost & Time <span class="subtle">({apiResult.craftingCost?.expansionStatus ?? 'N/A'})</span></h4>
					{#if apiResult.craftingCost?.expansionStatus && !apiResult.craftingCost.expansionStatus.startsWith('Failed') && apiResult.craftingCost.expansionStatus !== 'Yielded no ingredients'}
						<p>
							Total Craft Cost:
							<strong>{apiResult.craftingCost?.totalCost?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) ?? 'N/A'}</strong>
						</p>
						{#if apiResult.craftingCost?.ingredientCostErrors?.length > 0}
							<div class="error-box">
								<p>Ingredient Cost Issues:</p>
								<ul>{#each apiResult.craftingCost.ingredientCostErrors as err}<li>{err}</li>{/each}</ul>
							</div>
						{/if}
						<p>
							Bottleneck Fill Time:
							<strong>{apiResult.craftingCost?.bottleneckFillTime?.formatted ?? 'N/A'}</strong>
							{#if apiResult.craftingCost?.bottleneckFillTime?.bottleneckId}
								<span class="subtle">(Ingredient: {apiResult.craftingCost.bottleneckFillTime.bottleneckId})</span>
							{/if}
						</p>
						{#if apiResult.craftingCost?.bottleneckFillTime?.error}
							<p class="error-detail">Fill Time Error: {apiResult.craftingCost.bottleneckFillTime.error}</p>
						{/if}
						{#if apiResult.craftingCost?.ingredientFillErrors?.length > 0}
							<div class="error-box">
								<p>Ingredient Fill Time Issues:</p>
								<ul>{#each apiResult.craftingCost.ingredientFillErrors as err}<li>{err}</li>{/each}</ul>
							</div>
						{/if}

						{#if apiResult.craftingCost?.baseIngredients?.length > 0}
							<h5>Base Ingredients ({apiResult.craftingCost.baseIngredients.length}):</h5>
							<ul class="ingredient-list">
								{#each apiResult.craftingCost.baseIngredients as ing}
									<li>
										{ing.quantity.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 2 })} x {ing.id}:
										<span class="subtle">({ing.method}: {ing.cost?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) ?? 'ERR'})</span>
									</li>
								{/each}
							</ul>
						{/if}
					{:else}
						<p class="subtle">Crafting cost/time not applicable or calculation failed.</p>
					{/if}
				</section>

				<!-- Comparison Card -->
				<section class="card">
					<h4>Comparison</h4>
					<p>{apiResult.comparison?.message ?? 'N/A'}</p>
				</section>

				<!-- Instasell Time Card -->
				<section class="card">
					<h4>InstaSell Time (Final Product)</h4>
					<p>
						Est. Time to Sell: <strong>{apiResult.instaSellFillTime?.formatted ?? 'N/A'}</strong>
					</p>
					{#if apiResult.instaSellFillTime?.error}
						<p class="error-detail">Instasell Error: {apiResult.instaSellFillTime.error}</p>
					{/if}
				</section>
				<p class="subtle calculation-time">Server Calculation Time: {apiResult.calculationTimeMs}ms</p>
			</div>
		{:else}
            <!-- Initial state before first calculation -->
            <p class="status">Enter an Item ID and Quantity, then click Calculate.</p>
        {/if}
	</div>
</div>

<style>
	/* Styles specific to the calculator page */
	.calculator-content h2 {
		margin-top: 0;
        margin-bottom: 1.5rem;
		border-bottom: 1px solid #eee;
		padding-bottom: 0.75rem;
        color: #333;
	}
	.input-group {
		display: flex;
        flex-direction: column; /* Stack inputs vertically on small screens */
		gap: 0.8rem; /* Spacing between elements */
		margin-bottom: 2rem;
        padding: 1.5rem;
        border-radius: 8px;
        background-color: #f8f9fa; /* Light background for input area */
	}
     .input-group label {
        font-weight: 500;
        margin-bottom: 0.2rem;
        color: #555;
    }
	.input-group input {
		padding: 0.6rem 0.8rem; /* Slightly larger padding */
		border: 1px solid #ced4da; /* Standard bootstrap-like border */
		border-radius: 4px;
        width: 100%; /* Make inputs take full width */
        box-sizing: border-box; /* Include padding in width */
	}
     .input-group input:focus {
        border-color: #80bdff;
        outline: 0;
        box-shadow: 0 0 0 0.2rem rgba(0, 123, 255, 0.25);
    }
	.input-group button {
		padding: 0.7rem 1.2rem; /* Make button slightly larger */
		cursor: pointer;
		background-color: #007bff;
		color: white;
		border: none;
		border-radius: 4px;
		transition: background-color 0.15s ease-in-out;
        align-self: flex-start; /* Align to start */
        margin-top: 0.5rem;
	}
    .input-group button:hover:not(:disabled) {
        background-color: #0056b3;
    }
	.input-group button:disabled {
		background-color: #6c757d; /* Grey out when disabled */
		cursor: not-allowed;
	}

    /* Responsive input group */
    @media (min-width: 768px) {
        .input-group {
            flex-direction: row; /* Row layout on larger screens */
            align-items: flex-end; /* Align items to bottom */
        }
        .input-group label {
            flex: 1; /* Allow labels/inputs to grow */
             margin-bottom: 0;
        }
         .input-group input {
             width: auto; /* Allow input to size naturally */
        }
         .input-group button {
            flex-shrink: 0; /* Prevent button from shrinking */
            align-self: flex-end;
            margin-top: 0;
         }
    }


	.status-results {
		margin-top: 1.5rem;
	}
	.status {
		font-style: italic;
		padding: 1rem 1.5rem;
		border-radius: 4px;
		background-color: #e9ecef; /* Lighter grey */
        border: 1px solid #dee2e6;
        color: #495057;
	}
	.results {
		margin-top: 1rem;
	}
     .results h3 {
        color: #17a2b8; /* Teal color for result headers */
        margin-bottom: 1rem;
    }
	.card {
		border: 1px solid #e9ecef; /* Lighter border */
		border-radius: 6px; /* Slightly smaller radius */
		padding: 1rem 1.5rem;
		margin-bottom: 1.5rem;
		background-color: #ffffff; /* White background */
        box-shadow: 0 2px 4px rgba(0,0,0,0.04); /* Subtle shadow */
	}
	.card h4 {
		margin-top: 0;
		color: #0056b3;
		border-bottom: 1px solid #e0e0e0;
		padding-bottom: 0.5rem;
		margin-bottom: 1rem;
        font-size: 1.1em;
        font-weight: 600;
	}
	.card h5 {
		margin-top: 1.5rem;
		margin-bottom: 0.5rem;
		color: #5a5a5a;
        font-size: 1em;
	}
    .card p {
        margin-top: 0.5rem;
        margin-bottom: 0.5rem;
        line-height: 1.7; /* Slightly more spacing in paragraphs */
    }
    .card strong {
        color: #333;
    }
	.error {
		color: #dc3545; /* Bootstrap danger color */
		font-weight: 500; /* Slightly less bold */
		background-color: #f8d7da; /* Bootstrap danger background */
		border-color: #f5c6cb; /* Bootstrap danger border */
        border-left-width: 4px;
	}
	.warning {
		color: #856404;
		font-weight: normal;
		border: 1px solid #ffeeba;
		padding: 0.75rem 1.25rem;
		border-radius: 4px;
		background-color: #fff3cd;
		margin-bottom: 1.5rem; /* More space after warning */
	}
	.error-detail {
		color: #721c24; /* Darker red */
		font-size: 0.9em;
        margin-top: 0.3rem;
        padding-left: 0.5rem;
        border-left: 2px solid #f5c6cb;
	}
	.error-box {
		border: 1px solid #f5c6cb;
		background-color: #f8d7da;
		padding: 0.75rem 1rem;
		margin-top: 1rem;
		border-radius: 4px;
		font-size: 0.9em;
	}
	.error-box p {
		margin-top: 0;
		margin-bottom: 0.5rem;
		font-weight: bold;
		color: #721c24;
	}
	.error-box ul {
		margin: 0 0 0 1.2rem;
		padding: 0;
		list-style: disc;
		color: #5a5a5a;
	}
	.subtle {
		color: #6c757d; /* Bootstrap secondary text color */
		font-size: 0.9em;
	}
	.ingredient-list {
		list-style: none;
		padding-left: 0;
		font-size: 0.95em;
		max-height: 250px; /* Slightly taller */
		overflow-y: auto;
		border: 1px solid #dee2e6;
		padding: 0.75rem 1rem;
		border-radius: 4px;
		background-color: #f8f9fa; /* Light background */
        margin-top: 0.75rem;
	}
	.ingredient-list li {
		margin-bottom: 0.5rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px dashed #dee2e6; /* Dashed separator */
	}
	.ingredient-list li:last-child {
		border-bottom: none;
		margin-bottom: 0;
		padding-bottom: 0;
	}
	.calculation-time {
		margin-top: 1.5rem;
		text-align: right;
		color: #6c757d;
		font-size: 0.8em;
	}
</style>