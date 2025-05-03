Below is an outline of the revised logic flow and the formulas that should be used.
1. Base Item Identification

    Step: Expand the main item into its base ingredients.

    Output: A map of all base items with their required quantities.

    Note: This part is assumed to be working correctly.

2. Identify the Bottleneck Base Item

    Step: For each base item, compute the fill time using a top-level quantity of 1 and ignoring any extra term.

    Goal: Determine which base item has the longest fill time (i.e. the slowest one).

    Formula (without extra term):
    FTbase=20×1×factor(SellSize×SellFrequency−OrderSizeAverage×OrderFrequencyAverage)
    FTbase​=(SellSize×SellFrequency−OrderSizeAverage×OrderFrequencyAverage)20×1×factor​

    (or if the comparison branch chooses the denominator without the subtraction term)

    Result:

        Bottleneck Item: The one with the longest FTbaseFTbase​.

        Throughput of Base Items:
        Throughput=3600FTbase (bottleneck)
        Throughput=FTbase (bottleneck)​3600​

3. Determine the Pricing Source for the Main Item

    Step: Check if the main (top-level) item is using the buy_summary[0] price.

    Method:

        Compare the top-level price (determined by getTopLevelPrice()) with the buy price (getBuyPrice()).

        If they match (within an acceptable epsilon), then usingBuyPrice = true.

4. Compute the Final Fill Time for the Main Item

    Step: Now that we know whether the main item uses the buy_summary price, we update the fill time calculation for the main item.

    Set a variable qtymainqtymain​ (which is the number of main items needed; this value might be different from 1 if we are “solving for quantity” in profit calculations).

    If using buy_summary price:

    The new formula incorporates an extra term that accounts for the rate of "instant buys."

        Instant Buys Rate:
        instabuysPerHour=buymovingweek168
        instabuysPerHour=168buymovingweek​

        Final Fill Time (with extra term):
        FTfinal=20×(qtymain)×factor(SellSize×SellFrequency−OrderSizeAverage×OrderFrequencyAverage)+(qtymain×60)instabuysPerHour
        FTfinal​=(SellSize×SellFrequency−OrderSizeAverage×OrderFrequencyAverage)20×(qtymain​)×factor​+instabuysPerHour(qtymain​×60)​

    Else (if not using buy_summary price):

    The fill time remains as:
    FTfinal=20×(qtymain)×factor(SellSize×SellFrequency−OrderSizeAverage×OrderSizeAverage)
    FTfinal​=(SellSize×SellFrequency−OrderSizeAverage×OrderSizeAverage)20×(qtymain​)×factor​

    (or using the appropriate denominator branch, as already defined)

5. Profit Per Hour Calculation

    Step:

        Profit per Item:
        Profit per Item=Main Price−Total Base Cost
        Profit per Item=Main Price−Total Base Cost

        where
        Total Base Cost=∑i(Price of Base Itemi×Required Qtyi)Total Base Cost=∑i​(Price of Base Itemi​×Required Qtyi​)

        Profit per Hour:
        Profit per Hour=Profit per Item×Throughput (items per hour)
        Profit per Hour=Profit per Item×Throughput (items per hour)

    Throughput Determination:
    Use the bottleneck base item's fill time (computed without the extra term) to determine the maximum number of main items that can be processed in one hour.

Data Flow Summary

    Input:

        Main item name.

        Desired main item quantity (qtymainqtymain​).

        Market data (SellSummary, BuySummary, moving week values) and product metrics.

    Expansion:

        Expand the main item into its base ingredients with their required quantities.

    Base Bottleneck Calculation:

        For each base ingredient (using quantity 1), compute fill time (without the extra term).

        Identify the base item with the longest fill time.

        Calculate throughput as Throughput=3600Longest Fill TimeThroughput=Longest Fill Time3600​.

    Determine Price Source:

        Check if the main item is using the buy_summary price.

    Final Fill Time Calculation:

        If using buy_summary, compute fill time with the extra term:
        FTfinal=FTbase+(qtymain×60)(buymovingweek/168)
        FTfinal​=FTbase​+(buymovingweek/168)(qtymain​×60)​

        Otherwise, use the base fill time formula.

    Profit Calculation:

        Compute the profit per main item.

        Multiply by throughput (the processing rate determined by the bottleneck base ingredient) to get profit per hour.

Final Note

This flow means that the main item’s throughput is defined by the slowest (bottleneck) base ingredient when processed at a quantity of 1 (ignoring the extra term). Then, once you know the throughput, you re-calculate the main item’s fill time using the full formula (if needed) and determine profit per hour accordingly.

This revised logic should ensure that you correctly solve for the optimal quantity given the extra term is only applied when using the buy_summary price, and that the bottleneck is correctly determined from the base ingredients.
You said:
we don't actually need the throughput in step 3 of the data flow (for the base item) 

then we do 3600/ft_final, then multiply that by profit per item to get profit per hour. 

full code please

Explanation

    Expansion & Profit Calculation:
    The code first expands the main item to obtain its base cost and then computes profit per item as (main price − total base cost).

    Final Fill Time Calculation:
    Using the new function newComputeFillTime, the main item’s fill time is computed (with an extra term added if the item uses the buy_summary price).
    The extra term is calculated as:
    extra=(qty×60)(buymovingweek/168)
    extra=(buymovingweek/168)(qty×60)​

    This extra term is only added when using the buy_summary price.

    Profit per Hour:
    Throughput is computed as 3600ftfinalftfinal​3600​ and profit per hour is calculated as:
    Profit per Hour=Profit per Item×Throughput
    Profit per Hour=Profit per Item×Throughput

Feel free to adjust or extend any parts as needed for your application.
