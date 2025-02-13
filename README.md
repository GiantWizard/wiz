free money generator


2/12/2025

Key changes/fixes:

    Started updating README.md.

    Now includes an engine for the craft code.

    Logic now factors in inventory cycles (how many times you need to fill your inventory or sell cycles/inventory space) and divides profit per hour by the exponential function $x^(\frac{x}{2240})$ where $x$ is the number of inventory cycles.
        Ensures that high cycle high "profit" flips don't count (e.g. rough gemstones or enchanted items) as much as lower cycle similar profit flips.
        Cycle times are calculated by the sell cycles, which is how many times an item is crafted an hour.

    Remodeled sell time and fill times.
        Single item total fill time now multiplies by the total count of the item needed.
        Previously was using single item fill time, resulting in an inaccurate representation.
        Also added to sell order fill time (if buy order, otherwise 0).

    Recursive algorithim in json file now fixed.
        Previously was only 1 step crafts and didn't account for the recursive tree.

TODO:

    *Fix recipe tree (in terminal?) and json.
    Reconsider subitems based on crafted item with an efficiency score to reduce the inventory cycles (such as raw gemstones).
    Continue testing json exports for bugs.
    Don't forget to put new logic in the export engine!
    
