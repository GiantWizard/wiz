// src/routes/api/failed_items_report/+server.ts
import { json, type RequestHandler } from '@sveltejs/kit';
import type { KoyebFailedItemsReportResponse, ApiErrorResponse } from '$lib/types/koyeb'; // Assuming your types are in $lib/types/koyeb.ts

const KOYEB_FAILED_ITEMS_URL = 'https://wiz-backend-giantwizard-0f2a46ea.koyeb.app/failed_items_report.json'; // << UPDATED URL

export const GET: RequestHandler = async ({ fetch: svelteKitFetch }) => {
    const targetUrl = KOYEB_FAILED_ITEMS_URL;
    console.log(`[SK FailedItems Endpoint] Initiating GET request. Target URL: ${targetUrl}`);

    try {
        const response = await svelteKitFetch(targetUrl);
        console.log(`[SK FailedItems Endpoint] Received response from Koyeb. Status: ${response.status}, StatusText: ${response.statusText}`);

        if (!response.ok) {
            let errorBodyText = `Backend at ${targetUrl} responded with status: ${response.status}`;
            try {
                const backendError = await response.json() as ApiErrorResponse;
                if (backendError && backendError.error) {
                    errorBodyText = backendError.error;
                } else {
                    errorBodyText = `Backend error (status ${response.status}), could not parse JSON error body.`;
                    const rawTextAttempt = await response.text();
                    if(rawTextAttempt) errorBodyText = rawTextAttempt;
                }
            } catch (e) {
                console.warn(`[SK FailedItems Endpoint] Failed to parse JSON error body from Koyeb, attempting to read as text. Error: ${e instanceof Error ? e.message : String(e)}`);
                try {
                    const textError = await response.text();
                    if (textError) {
                        errorBodyText = textError;
                    } else {
                        errorBodyText = `Backend error (status ${response.status}), could not retrieve error body details.`;
                    }
                } catch (finalE) {
                    console.warn(`[SK FailedItems Endpoint] Also failed to read error body as text. Error: ${finalE instanceof Error ? finalE.message : String(finalE)}`);
                     errorBodyText = `Backend error (status ${response.status}), error body unreadable.`;
                }
            }

            console.error(`[SK FailedItems Endpoint] Error fetching from Koyeb. Target: ${targetUrl}, Status: ${response.status}, Details: ${errorBodyText}`);
            return json(
                { error: `Failed to fetch failed items report: ${errorBodyText}` },
                { status: response.status > 0 && response.status < 600 ? response.status : 500 }
            );
        }

        let data;
        try {
            data = await response.json() as KoyebFailedItemsReportResponse | ApiErrorResponse;
            console.log("[SK FailedItems Endpoint] Successfully fetched and parsed JSON from Koyeb.");
        } catch (e) {
            const parseErrorMessage = e instanceof Error ? e.message : String(e);
            console.error(`[SK FailedItems Endpoint] Successfully fetched from Koyeb (Status ${response.status}), but failed to parse JSON response. Error: ${parseErrorMessage}`);
            return json(
                { error: `Failed to parse successful response from backend: ${parseErrorMessage}` },
                { status: 502 }
            );
        }

        if ('error' in data && typeof (data as ApiErrorResponse).error === 'string') {
             console.warn('[SK FailedItems Endpoint] Koyeb responded OK, but payload indicates an application-level error:', data);
             return json(data, { status: 400 });
        }

        console.log("[SK FailedItems Endpoint] Data appears valid. Returning data to client.");
        return new Response(JSON.stringify(data), {
            headers: {
                'Content-Type': 'application/json',
                'Cache-Control': 'public, max-age=15, s-maxage=15, stale-while-revalidate=30',
            },
        });

    } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Unknown internal server error';
        console.error(`[SK FailedItems Endpoint] Critical error during fetch operation to ${targetUrl}: ${errorMessage}`, error);
        return json(
            { error: 'Internal server error in proxy function: ' + errorMessage },
            { status: 500 }
        );
    }
};