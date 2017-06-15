package main

// Connects to a P1 smart meter via serial port and makes the received
// telegrams with the data available via a webservice.

import (
	"encoding/json"
	"flag"
	"github.com/bwesterb/go-dsmrp1"
	"log"
	"net/http"
	"sync"
)

func main() {
	var serialDev string
	var host string
	var telegram *dsmrp1.Telegram
	var telegramLock sync.Mutex

	flag.StringVar(&serialDev, "serial", "/dev/P1",
		"path to serial port")
	flag.StringVar(&host, "host", "127.0.0.1:1121",
		"host to bind to for webserver")

	flag.Parse()

	m, err := dsmrp1.NewMeter(serialDev)
	if err != nil {
		log.Fatalf("Failed to create meter: %v", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		telegramLock.Lock()
		defer telegramLock.Unlock()
		s, _ := json.Marshal(telegram)
		w.Write(s)
	})

	go func() {
		for w := range m.C {
			telegramLock.Lock()
			telegram = w
			telegramLock.Unlock()
		}
	}()

	log.Fatal(http.ListenAndServe(host, nil))
}
