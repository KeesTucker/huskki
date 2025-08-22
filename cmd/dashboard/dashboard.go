package main

import (
	"huskki/config"
	"huskki/drivers"
	"huskki/ecu"
	"huskki/events"
	"huskki/web/handlers"
	"log"
)

func main() {
	flags, serialFlags, replayFlags := config.GetFlags()

	eventHub := events.NewHub()

	isReplay := replayFlags.Path != ""

	// Create the correct driver
	var driver drivers.Driver
	if isReplay {
		driver = drivers.NewReplayer(replayFlags, &ecu.K701{}, eventHub)
	} else {
		driver = drivers.NewArduino(serialFlags, &ecu.K701{}, eventHub)
	}

	// Start up the driver
	err := driver.Init()
	if err != nil {
		log.Printf("couldn't init driver: %s", err)
		return
	}

	go func() {
		err = driver.Run()
		if err != nil {
			log.Printf("error running driver: %s", err)
		}
	}()

	// Initialise UI
	dashboard, err := web.NewDashboard()
	if err != nil {
		log.Fatalf("couldn't create dashboard: %v", err)
	}

	// Initialise Server
	server := web.NewServer(dashboard, eventHub)
	err = server.Start(flags.Addr)
	if err != nil {
		log.Fatalf("couldn't start server: %v", err)
	}
}
