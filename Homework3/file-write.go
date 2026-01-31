package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

func main() {
	const iterations = 100000
	line := "This is a test line\n"

	// -------- Unbuffered write --------
	f1, err := os.Create("unbuffered.txt")
	if err != nil {
		panic(err)
	}

	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, err := f1.Write([]byte(line))
		if err != nil {
			panic(err)
		}
	}
	f1.Close()
	unbufferedTime := time.Since(start)

	// -------- Buffered write --------
	f2, err := os.Create("buffered.txt")
	if err != nil {
		panic(err)
	}

	writer := bufio.NewWriter(f2)
	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := writer.WriteString(line)
		if err != nil {
			panic(err)
		}
	}
	writer.Flush()
	f2.Close()
	bufferedTime := time.Since(start)

	// -------- Results --------
	fmt.Println("Unbuffered write time:", unbufferedTime)
	fmt.Println("Buffered write time:  ", bufferedTime)
}
