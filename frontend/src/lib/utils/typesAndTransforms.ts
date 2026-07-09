// /Users/a/Downloads/wiz/frontend/src/lib/utils/typesAndTransforms.ts

// --- New JSON Structure Interfaces ---
export interface NewAcquisition {
  quantity: number;
  best_cost: number;
  associated_cost: number;
  method: string;
  delta: number;
}

export interface NewRecipeTreeNode {
  item_name: string;
  quantity_needed: number;
  quantity_per_craft: number;
  num_crafts: number;
  is_base_component: boolean;
  ingredients?: NewRecipeTreeNode[];
  acquisition?: NewAcquisition;
  depth: number;
  max_sub_tree_depth: number;
}

export interface NewTopLevelItem {
  item_name: string;
  max_feasible_quantity: number;
  cost_at_optimal_qty: number;
  revenue_at_optimal_qty: number;
  max_profit: number;
  total_cycle_time_at_optimal_qty: number;
  acquisition_time_at_optimal_qty: number; // Added based on detail page usage
  sale_time_at_optimal_qty: number;         // Added based on detail page usage
  bottleneck_ingredient_qty: number;
  calculation_possible: boolean;
  recipe_tree: NewRecipeTreeNode;
  max_recipe_depth: number;
}

// --- Transformed Structure for RecipeTree.svelte ---
export interface TransformedTree {
  item?: string;
  ingredients?: TransformedIngredient[];
}

export interface TransformedIngredient {
  ingredient: string;
  total_needed: number;
  buy_method?: string;
  cost_per_unit?: number;
  sub_breakdown?: TransformedTree;
}

// --- Raw Material Report for Detail Page ---
export interface RawMaterialReport {
  ingredient: string;
  total_needed: number;
  cost_per_unit?: number;
  buy_method?: string;
}

// --- Price Data for Charts (used in detail page and AveragePriceChart) ---
export interface PriceHistory { // Renamed from PriceRecord to avoid conflict if any
  buy: number;
  sell: number;
  timestamp: string;
}
export interface AvgPriceData {
  item: string;
  history: PriceHistory[];
}


// --- Transformation Functions ---
export function simplifiedTransform(node: NewRecipeTreeNode): TransformedTree {
  const transformed: TransformedTree = {
    item: node.item_name,
  };

  if (node.ingredients && node.ingredients.length > 0) {
    transformed.ingredients = node.ingredients.map(ingNode => {
      const tIng: TransformedIngredient = {
        ingredient: ingNode.item_name,
        total_needed: ingNode.quantity_needed,
      };

      if (ingNode.acquisition) {
        tIng.buy_method = ingNode.acquisition.method;
        if (ingNode.acquisition.quantity > 0 && ingNode.acquisition.best_cost != null) {
          tIng.cost_per_unit = ingNode.acquisition.best_cost / ingNode.acquisition.quantity;
        } else {
          tIng.cost_per_unit = 0; // Default if no valid cost
        }
      }

      if (!ingNode.is_base_component && ingNode.ingredients && ingNode.ingredients.length > 0) {
         tIng.sub_breakdown = simplifiedTransform(ingNode);
      }
      return tIng;
    });
  }
  return transformed;
}

export function getRawMaterialsFromNewTree(rootNode: NewRecipeTreeNode): RawMaterialReport[] {
  const rawMaterialsMap: Map<string, RawMaterialReport> = new Map();

  function findRaws(currentNode: NewRecipeTreeNode, requiredQuantityOfCurrentNodeForOneFinalProduct: number) {
    if (currentNode.is_base_component) {
      const existing = rawMaterialsMap.get(currentNode.item_name);
      const quantityToAdd = requiredQuantityOfCurrentNodeForOneFinalProduct;

      if (existing) {
        existing.total_needed += quantityToAdd;
      } else {
        const newRaw: RawMaterialReport = {
          ingredient: currentNode.item_name,
          total_needed: quantityToAdd,
        };
        if (currentNode.acquisition) {
          newRaw.buy_method = currentNode.acquisition.method;
          if (currentNode.acquisition.quantity > 0 && currentNode.acquisition.best_cost != null) {
            newRaw.cost_per_unit = (currentNode.acquisition.best_cost / currentNode.acquisition.quantity);
          }
        }
        rawMaterialsMap.set(currentNode.item_name, newRaw);
      }
      return;
    }

    if (currentNode.ingredients) {
      for (const ingredientNode of currentNode.ingredients) {
        findRaws(ingredientNode, ingredientNode.quantity_needed * requiredQuantityOfCurrentNodeForOneFinalProduct);
      }
    }
  }

  if (rootNode.ingredients) {
    for (const firstLevelIngredient of rootNode.ingredients) {
      findRaws(firstLevelIngredient, firstLevelIngredient.quantity_needed);
    }
  } else if (rootNode.is_base_component) {
     findRaws(rootNode, 1);
  }

  return Array.from(rawMaterialsMap.values());
}

export function toTitleCase(str?: string): string {
    if (!str) return '';
    return str
      .replace(/_/g, ' ')
      .toLowerCase()
      .replace(/\b(\w)/g, (_match, char) => char.toUpperCase());
}

export function abbreviateNumber(value?: number, decimals: number = 1): string {
    if (value == null || isNaN(value)) return '0';
    const absValue = Math.abs(value);
    if (absValue < 1000) return value.toFixed(value % 1 === 0 ? 0 : decimals); 
    
    const tier = Math.floor(Math.log10(absValue) / 3);
    if (tier === 0) return value.toFixed(value % 1 === 0 ? 0 : decimals);

    const suffix = ['', 'K', 'M', 'B', 'T'][tier];
    const scale = Math.pow(10, tier * 3);
    const scaled = value / scale;
    
    return scaled.toFixed(decimals).replace(/\.0+$/, '') + suffix;
}

export function formatNumberSimple(num?: number, decimals: number = 1): string {
    if (num === null || num === undefined || isNaN(num)) return '0';
    const numForFormatting = Number(num);
    const formatted = numForFormatting.toLocaleString('en-US', {
      minimumFractionDigits: numForFormatting % 1 === 0 ? 0 : decimals,
      maximumFractionDigits: decimals
    });
    return formatted.replace(/\.0+$/, '');
}

// Helper for AveragePriceChart's rounding, if different from abbreviateNumber
export function formatLargeNumberForChart(num: number): string {
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