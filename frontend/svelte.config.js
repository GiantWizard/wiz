import adapter from '@sveltejs/adapter-auto';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
    preprocess: vitePreprocess(),
    kit: {
        adapter: adapter(),
        alias: {
            // Example: if you wanted to use '@components' for '$lib/components'
            // '@components': 'src/lib/components', 
            // But usually '$lib' is enough and automatically handled
            // If you had other custom paths, define them here:
            // 'my-custom-alias': 'src/path/to/custom/folder'
        }
        // Ensure your kit.files.assets points to 'static' if that's where your JSON files are.
        // This is usually the default.
        // files: {
        //  assets: 'static'
        // }
    }
};

export default config;