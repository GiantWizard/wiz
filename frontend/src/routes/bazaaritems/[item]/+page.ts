import type { PageLoad } from './$types';
import type { BazaarItem } from '$lib/types';

export const load = (async ({ params, fetch }) => {
  console.log("Load function START. Received params:", params);

  // Decode the parameter (which is the item name)
  const decodedItem = decodeURIComponent(params.item);
  console.log("Decoded item name:", decodedItem);

  // Fetch the JSON file from the static folder
  try {
    const res = await fetch('/top_40_bazaar_crafts.json');
    console.log("Fetch response status:", res.status);

    if (!res.ok) {
      console.error("Error fetching JSON. Status:", res.status);
      throw new Error('Could not fetch profitable items data');
    }

    const items = await res.json() as BazaarItem[];
    console.log("Items loaded from JSON:", items);

    // Search for the item using the decoded name
    const foundItem = items.find((i: BazaarItem) => i.item === decodedItem);
    console.log("Search result for item", decodedItem, ":", foundItem);

    if (!foundItem) {
      console.error("Item not found in JSON:", decodedItem);
      throw new Error(`Item ${decodedItem} not found`);
    }

    return {
      item: foundItem
    };
  } catch (error) {
    console.error("Load function caught an error:", error);
    throw error;
  }
}) satisfies PageLoad; 