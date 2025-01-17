package main

import (
	"log"
	"math"
	"time"
)

func goBenchmark() {
	log.Println("Starting Go benchmark")
	startTime := time.Now()
	result := 0.0
	for i := 1; i <= 100_000_000; i++ { // 100 million iterations
		result += math.Sqrt(float64(i)) * math.Sin(float64(i)) * math.Log(float64(i)+1) *
			math.Cos(float64(i)) * float64(i%1000) * math.Tan(float64(i%360)) *
			math.Exp(-float64(i%100)) / (math.Atan(float64(i%180)) + 1)
	}
	elapsedTime := time.Since(startTime)
	log.Printf("Go benchmark completed in %.2f seconds.\n", elapsedTime.Seconds())
}

func main() {
	goBenchmark()
}
