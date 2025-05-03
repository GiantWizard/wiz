// getRefreshRate computes and returns the base price (ps), the Initial Fill (IF), and refresh rate (RR)
// for the given item and quantity. If no metric exists, returns a very high cost.
func getRefreshRate(itemName string, qty int, metrics map[string]MarketMetric) (float64, float64, int) {
	metric, ok := metrics[itemName]
	if !ok {
		log.Printf("No market metric for '%s'", itemName)
		return math.MaxFloat64, 0, 1
	}

	ps := metric.PS
	ss := metric.SellSize
	sf := metric.SellFrequency
	os := metric.OrderSize
	of := metric.OrderFrequency

	RoF := ss * sf
	iino := os * of
	IF := (sf - of) * ss
	// Clamp IF to 0 if negative; if IF is 0, set RR to 1.
	if IF <= 0 {
		IF = 0
		return ps, IF, 1
	}

	var RR int
	if RoF >= iino {
		RR = 1
	} else {
		RR = int(math.Ceil(float64(qty) / IF))
	}
	return ps, IF, RR
}
