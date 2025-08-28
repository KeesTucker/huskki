package config

import (
	"flag"
)

type DriverType string

const (
	Replay    DriverType = "replay"
	Arduino   DriverType = "arduino"
	SocketCAN DriverType = "socket-can"
)

type Flags struct {
	Driver DriverType
	Addr   string
}

type SerialFlags struct {
	SerialPort string
	BaudRate   int
}

type ReplayFlags struct {
	Path       string
	Speed      float64
	Loop       bool
	SkipFrames int
}

type SocketCANFlags struct {
	SocketCanAddr string
}

const DEFAULT_BAUD_RATE = 115200

func GetFlags() (*Flags, *SerialFlags, *ReplayFlags, *SocketCANFlags) {
	flags := &Flags{}
	var driverStr string
	flag.StringVar(&driverStr, "driver", "socket-can", "driver type to use to communicate with vehicle")
	flag.StringVar(&flags.Addr, "addr", ":8080", "http listen address")

	serial := &SerialFlags{}
	flag.StringVar(&serial.SerialPort, "serial-port", "auto", "serial device path or 'auto'")
	flag.IntVar(&serial.BaudRate, "baud", DEFAULT_BAUD_RATE, "baud rate")

	replay := &ReplayFlags{}
	flag.StringVar(&replay.Path, "replay", "", "Path to .bin to replay")
	flag.Float64Var(&replay.Speed, "replay-speed", 1.0, "Replay speed multiplier (0 = as fast as possible)")
	flag.BoolVar(&replay.Loop, "replay-loop", false, "Loop replay at EOF")
	flag.IntVar(&replay.SkipFrames, "replay-skip-frames", 0, "Skips X amount of frames from start")

	socketCAN := &SocketCANFlags{}
	flag.StringVar(&socketCAN.SocketCanAddr, "socket-can-address", "can0", "Socket CAN bus address")

	flag.Parse()

	flags.Driver = DriverType(driverStr)

	return flags, serial, replay, socketCAN
}
