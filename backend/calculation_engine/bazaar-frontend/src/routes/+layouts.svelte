<script lang="ts">
	// Optional: Import global styles if you create an app.css file
	// import '../app.css';
</script>

<div class="app-container">
	<header>
		<h1>Bazaar Calculation Tool</h1>
	</header>

	<main>
		<!-- Page content (+page.svelte) goes here -->
		<slot />
	</main>

	<footer>
		<p>© {new Date().getFullYear()} Bazaar Calc</p>
	</footer>
</div>

<style>
	.app-container {
		display: flex;
		flex-direction: column;
		min-height: 100vh;
		max-width: 960px;
		margin: 0 auto;
		padding: 0 1rem;
	}

	header {
		background-color: #f4f7f6;
		padding: 0.8rem 0;
		text-align: center;
		border-bottom: 1px solid #dde;
		margin-bottom: 1.5rem;
	}

	header h1 {
		margin: 0;
		color: #2c3e50;
		font-weight: 500;
	}

	main {
		flex-grow: 1;
		padding-bottom: 2rem;
	}

	footer {
		text-align: center;
		margin-top: 2.5rem;
		padding: 1.5rem 0;
		border-top: 1px solid #eee;
		color: #888;
		font-size: 0.85em;
	}

	/* Basic global styles */
	:global(body) {
		margin: 0;
		font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen,
			Ubuntu, Cantarell, 'Open Sans', 'Helvetica Neue', sans-serif;
		line-height: 1.65;
		color: #34495e;
		background-color: #ffffff;
	}

	 :global(h1, h2, h3, h4, h5, h6) {
		color: #2c3e50;
        font-weight: 600;
	}

	:global(input), :global(button) {
		font-family: inherit;
		font-size: inherit;
	}
</style>