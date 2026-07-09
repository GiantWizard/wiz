export interface BazaarItem {
  item: string;
  profit_per_hour: number;
  crafting_savings: number;
  sell_price: number;
  crafting_cost: number;
  [key: string]: any; // For other properties
}
