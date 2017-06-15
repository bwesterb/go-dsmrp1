package main

// Connects a P1 smart meter via serial port and prints the parsed
// telegrams as JSON objects.

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bwesterb/go-dsmrp1"
	"log"
)

func main() {
	var serialDev string

	flag.StringVar(&serialDev, "serial", "/dev/P1",
		"path to serial port")

	flag.Parse()

	m, err := dsmrp1.NewMeter(serialDev)
	if err != nil {
		log.Fatalf("Failed to create meter: %v", err)
	}

	for w := range m.C {
		s, _ := json.MarshalIndent(w, "", "  ")
		fmt.Println(string(s))
	}
}
