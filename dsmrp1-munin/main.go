package main

// Munin plugin for electricity and gas data provided by the dsmrp1d daemon.

// TODO allow configuration of dsmrp1d host

import (
	"encoding/json"
	"fmt"
	"github.com/bwesterb/go-dsmrp1"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

func main() {
	var url string = "http://localhost:1121"
	if len(os.Args) == 1 {
		var telegram *dsmrp1.Telegram
		resp, err := http.Get(url)
		if err != nil {
			log.Fatalf("Could not connect to dsmrp1d at %s", url)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Failed to read response: %v", err)
		}
		err = json.Unmarshal(body, &telegram)
		if err != nil {
			log.Fatalf("Failed to parse telegram %v", err)
		}
		if telegram == nil {
			log.Fatal("No data, yet")
		}

		e := telegram.Electricity

		kWh := e.KWh + e.KWhLow - e.KWhOut - e.KWhOutLow
		dm3 := telegram.Gas.LastRecord.Value

		fmt.Println("multigraph p1_kWh")
		fmt.Printf("kWh.value %d\n", int64(kWh*1000*60*60))
		fmt.Println("")
		fmt.Println("multigraph p1_dm3")
		fmt.Printf("dm3.value %d\n", int64(dm3*1000))
		return
	}

	if os.Args[1] == "config" {
		fmt.Println("multigraph p1_kWh")
		fmt.Println("graph_title Electricity usage")
		fmt.Println("graph_vlabel Watt")
		fmt.Println("graph_category P1")
		fmt.Println("kWh.label Watt")
		fmt.Println("kWh.type DERIVE")
		fmt.Println("")
		fmt.Println("multigraph p1_dm3")
		fmt.Println("graph_title gas usage")
		fmt.Println("graph_vlabel dm3/h")
		fmt.Println("graph_period hour")
		fmt.Println("graph_category P1")
		fmt.Println("dm3.label dm3/h")
		fmt.Println("dm3.type DERIVE")
		return
	}

	if os.Args[1] == "autoconf" {
		fmt.Println("yes")
		return
	}

	os.Exit(-1)
}
