package main

import (
	"fmt"
	"runtime"
	"time"
)

func pingPong(roundTrips int) time.Duration {
	ch := make(chan struct{})
	done := make(chan struct{})

	start := time.Now()

	// Goroutine A
	go func() {
		for i := 0; i < roundTrips; i++ {
			ch <- struct{}{}
			<-ch
		}
		done <- struct{}{}
	}()

	// Goroutine B
	go func() {
		for i := 0; i < roundTrips; i++ {
			<-ch
			ch <- struct{}{}
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	return time.Since(start)
}

func main() {
	const roundTrips = 1_000_000

	// Case 1: force single OS thread
	runtime.GOMAXPROCS(1)
	d1 := pingPong(roundTrips)
	avg1 := d1.Seconds() / float64(2*roundTrips)
	fmt.Println("GOMAXPROCS(1) total:", d1, "avg per handoff:", avg1*1e9, "ns")

	// Case 2: allow multiple OS threads
	runtime.GOMAXPROCS(runtime.NumCPU())
	d2 := pingPong(roundTrips)
	avg2 := d2.Seconds() / float64(2*roundTrips)
	fmt.Println("GOMAXPROCS(N) total:", d2, "avg per handoff:", avg2*1e9, "ns")
}
