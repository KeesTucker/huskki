package main

import (
	"huskki/config"
	"huskki/drivers"
	"huskki/ecus"
	"huskki/web/handlers"
	"log"
)

func main() {
	flags, serialFlags, replayFlags, socketCANFlags := config.GetFlags()

	// Create the correct driver
	var driver drivers.Driver
	switch flags.Driver {
	case config.Arduino:
		driver = drivers.NewArduino(serialFlags, &ecus.K701{})
	case config.SocketCAN:
		driver = drivers.NewSocketCAN(socketCANFlags, &ecus.K701{})
	case config.Replay:
		driver = drivers.NewReplayer(replayFlags, &ecus.K701{})
	default:
		log.Fatalf("unsupported driver type: %s", flags.Driver)
		return
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
	server := web.NewServer(dashboard)
	err = server.Start(flags.Addr)
	if err != nil {
		log.Fatalf("couldn't start server: %v", err)
	}
}
