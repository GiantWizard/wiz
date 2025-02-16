import type { PageLoad } from './$types';
import type { BazaarItem } from '$lib/types';

export const load = (async ({ params, fetch }) => {
  // Decode the parameter (which is the item name)
  const decodedItem = decodeURIComponent(params.item);

  try {
    // Fetch the JSON file from the static folder
    const res = await fetch('/top_40_bazaar_crafts.json');
    if (!res.ok) {
      throw new Error('Could not fetch profitable items data');
    }

    const items = (await res.json()) as BazaarItem[];

    // Search for the item using the decoded name
    const foundItem = items.find((i: BazaarItem) => i.item === decodedItem);
    if (!foundItem) {
      throw new Error(`Item ${decodedItem} not found`);
    }

    return {
      item: foundItem
    };
  } catch (error) {
    throw error;
  }
}) satisfies PageLoad;
