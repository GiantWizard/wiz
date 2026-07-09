// wiz/frontend/svelte.config.js
// import adapter from '@sveltejs/adapter-auto'; // Remove or comment out
import adapter from '@sveltejs/adapter-cloudflare'; // USE THIS EXPLICITLY
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			// Optional: configure routes for Pages Functions
			// See https://kit.svelte.dev/docs/adapter-cloudflare#options-routes
			// routes: {
			//  include: ['/*'], // Which routes are handled by Functions
			//  exclude: ['/favicon.png'] // Which routes are purely static
			// }
		})
	}
};

export default config;