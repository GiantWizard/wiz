free money generator


2/12/2025

Key changes/fixes:

Started updating README.md.

Now includes an engine for the craft code.

Logic now factors in inventory cycles (how many times you need to fill your inventory or sell cycles/inventory space) and divides profit per hour by the exponential function $x^\frac{x}{2240}$ where $x$ is the number of inventory cycles.
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

2/13/2025

Key changes/fixes:

Fixed several bugs with the website (still localhosted).
Recipe tree displays correctly with step length.

TODO:

Update UI and make it more clean.
Include more details in json file.
Manage where exported files is being pushed/frontend is calling from.
Put new logic in export engine.
Factor in potential inventory cycles into calculations.

2/15/2025

Key changes/fixes:

Started making UI, landing page made. 
Solidified color scheme.

TODO:

*FIX LOGIC WHEN $x^\frac{x}{2240}, x < 1$.
Update buttons and functions, as well as the profit page.
Manage export files.
Update export engine.
Factor in potential inventory cycles.
Update description in landing.

2/16/2025

<<<<<<< HEAD
Key changes/fixes:
=======
Key changes/fixes
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee

Improved item detailed view UI.
Adding pointers to subitems in recipe tree.
Shifting around direction and placement of information in recipe tree.

TODO:

*FIX LOGIC WHEN $x^\frac{x}{2240}, x < 1$.
Change pointers into arcs, main accent line should end at the last pointer.
Swap position of multiplier and image.
Standardize space between top level item and its image and subitems and their images/multipler.
Manage export files.
Update export engine.
Factor in potential inventory cycles.
Update description in landing.

2/17/2025

<<<<<<< HEAD
Key changes/fixes:

=======
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
Fixed arc pointers in recipe tree.
Swaped position of multiplier and image.
Standardized space between top level item and its image and subitems and their images/multipler.

TODO:

*FIX LOGIC WHEN $x^\frac{x}{2240}, x < 1$.
Main accent line should end at the last pointer.
Manage export files.
Update export engine.
Factor in potential inventory cycles.
Update description in landing.

2/18/2025

<<<<<<< HEAD
Key changes/fixes:

=======
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
Finished recipe tree component.
Updated landing page.

TODO:

Add graphs for processing in recipe tree.
<<<<<<< HEAD
*FIX LOGIC WHEN $x^\frac{x}{2240}, x < 1$.
=======
Return more information from backend*FIX LOGIC WHEN $x^\frac{x}{2240}, x < 1$.
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
Main accent line should end at the last pointer.
Manage export files.
Update export engine.
Factor in potential inventory cycles.
<<<<<<< HEAD

2/22/2025

Key changes/fixes:

Added and finished graph in item overview, complete with pointers.
Fully finished recipe display.

TODO:

Round off % Flip and sell fill time.
*FIX LOGIC WHEN $x^\frac{x}{2240}, x < 1$.
Manage export files.
Update export engine.
Factor in potential inventory cycles.
Organize graphs.
=======
Give basic bazaar overview of each item in /bazaari
>>>>>>> fce51939aafc78a464aa367c4a5df48dc99280ee
