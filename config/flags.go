package config

import (
	"flag"
)

type Flags struct {
	Addr string
}

type SerialFlags struct {
	Port     string
	BaudRate int
}

type ReplayFlags struct {
	Path       string
	Speed      float64
	Loop       bool
	SkipFrames int
}

const DEFAULT_BAUD_RATE = 115200

func GetFlags() (*Flags, *SerialFlags, *ReplayFlags) {
	flags := &Flags{}
	flag.StringVar(&flags.Addr, "addr", ":8080", "http listen address")

	serial := &SerialFlags{}
	flag.StringVar(&serial.Port, "port", "auto", "serial device path or 'auto'")
	flag.IntVar(&serial.BaudRate, "baud", DEFAULT_BAUD_RATE, "baud rate")

	replay := &ReplayFlags{}
	flag.StringVar(&replay.Path, "replay", "", "Path to .bin to replay")
	flag.Float64Var(&replay.Speed, "replay-speed", 1.0, "Replay speed multiplier (0 = as fast as possible)")
	flag.BoolVar(&replay.Loop, "replay-loop", false, "Loop replay at EOF")
	flag.IntVar(&replay.SkipFrames, "replay-skip-frames", 0, "Skips X amount of frames from start")

	flag.Parse()

	return flags, serial, replay
}
