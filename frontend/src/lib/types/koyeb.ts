// src/lib/types/koyeb.ts

// --- For optimizer_results.json ---
export interface KoyebOptimizationSummary {
    run_timestamp: string;
    api_last_updated_timestamp?: string;
    total_items_considered: number;
    items_successfully_calculated: number;
    items_with_calculation_errors: number;
    max_allowed_cycle_time_seconds: number;
    max_initial_search_quantity: number;
}

export interface KoyebOptimizedItemResult {
    item_name: string;
    calculation_possible: boolean;
    error_message?: string;
}

export interface KoyebOptimizerResponse {
    summary: KoyebOptimizationSummary;
    results: KoyebOptimizedItemResult[];
}

// --- For failed_items_report.json ---
export interface KoyebFailedItemDetail {
    item_name: string;
    error_message?: string;
}

export type KoyebFailedItemsReportResponse = KoyebFailedItemDetail[];

// --- For generic error responses from the proxy or Koyeb ---
export interface ApiErrorResponse {
    error: string;
    details?: any; // Optional additional details
}