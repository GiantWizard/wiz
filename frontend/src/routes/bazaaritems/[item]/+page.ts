// src/routes/bazaaritems/[item]/+page.ts
import type { PageLoad } from './$types';
import type { NewTopLevelItem } from '$lib/utils/typesAndTransforms'; // Ensure this path is correct
import { error as svelteKitError } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, fetch }) => {
  const decodedItemName = decodeURIComponent(params.item);
  console.log(`\n--- FULL LOAD FUNCTION in +page.ts STARTED ---`);
  console.log(`[FULL LOAD +page.ts] Param item: '${params.item}', Decoded: '${decodedItemName}'`);

  try {
    const res = await fetch('/optimizer_results.json'); 
    console.log(`[FULL LOAD +page.ts] Fetch status for /optimizer_results.json: ${res.status}`);

    if (!res.ok) {
        const errorText = await res.text().catch(() => "Could not read error response body");
        console.error(`[FULL LOAD +page.ts ERROR] Failed to fetch /optimizer_results.json: ${res.status} ${res.statusText}. Body: ${errorText}`);
        throw svelteKitError(res.status, `Could not fetch profitable items data: ${res.statusText}`);
    }
    
    const jsonData: { summary: any; results: NewTopLevelItem[] } = await res.json(); 

    if (!jsonData || !Array.isArray(jsonData.results)) {
        console.error("[FULL LOAD +page.ts ERROR] Fetched data does not contain a 'results' array or is malformed:", jsonData);
        throw svelteKitError(500, 'Fetched item data.results is not in the expected array format.');
    }
    
    const itemsArray: NewTopLevelItem[] = jsonData.results; 
    
    console.log(`[FULL LOAD +page.ts] Searching for item: '${decodedItemName}' in an array of ${itemsArray.length} items.`);
    
    const foundItem = itemsArray.find(i => i && i.item_name && i.item_name === decodedItemName); 

    if (!foundItem) {
        console.error(`[FULL LOAD +page.ts ERROR THROWN] Item '${decodedItemName}' NOT FOUND in itemsArray.`);
        // Optionally log available names for debugging if item not found
        if (itemsArray.length < 20) { // Avoid logging excessively large arrays
             console.log('[FULL LOAD +page.ts INFO] All available item names:', itemsArray.map(i => i.item_name));
        } else {
             console.log('[FULL LOAD +page.ts INFO] First 20 item names:', itemsArray.slice(0, 20).map(i => i.item_name));
        }
        throw svelteKitError(404, `Item "${decodedItemName}" not found.`);
    }
    
    console.log(`[FULL LOAD +page.ts SUCCESS] Item '${decodedItemName}' found and returning actual data.`);
    console.log(`--- FULL LOAD FUNCTION in +page.ts END ---`);
    return {
      item: foundItem // Return the ACTUAL found item
    };

  } catch (errorCaught: any) {
    console.error("[FULL LOAD +page.ts CATCH BLOCK] Error during load:", errorCaught.message || errorCaught);
    console.log(`--- FULL LOAD FUNCTION in +page.ts END WITH ERROR ---`);
    if (typeof errorCaught === 'object' && errorCaught !== null && 'status' in errorCaught && 'message' in errorCaught) {
       throw errorCaught; 
    }
    throw svelteKitError(500, (errorCaught instanceof Error ? errorCaught.message : 'Unknown error loading item details from +page.ts'));
  }
};