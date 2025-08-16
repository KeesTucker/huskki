package main

import (
	"bufio"
	"errors"
	"huskki/hub"
	"io"
	"log"
	"os"
	"time"
)

type replayer struct {
	*ReplayFlags
}

func newReplayer(flags *ReplayFlags) replayer {
	return replayer{
		flags,
	}
}

func (r replayer) run(h *hub.EventHub) error {
	for {
		if err := r.playOnce(h); err != nil {
			return err
		}
		if !r.Loop {
			break
		}
	}
	return nil
}

func (r replayer) playOnce(h *hub.EventHub) error {
	file, err := os.Open(r.Path)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Printf("couldn't close file: %s", err)
		}
	}(file)

	bufferReader := bufio.NewReaderSize(file, 1<<20)

	var (
		first  = true
		prevMS int64
	)

	frameIndex := 0
	for {
		frame, err := readOneFrame(bufferReader)
		if err != nil {
			if err == io.EOF {
				log.Println("end of replay")
				return nil
			}
			// skip crc errors
			if errors.Is(err, badCrcErr) {
				// Not skipping atm cause crc was broken in early logs.
				//continue
			}
			return err
		}

		if frameIndex < r.SkipFrames {
			frameIndex++
			continue
		}

		if first {
			first = false
			prevMS = int64(frame.Millis)
		}

		if r.Speed > 0 {
			delta := time.Duration(int64(frame.Millis) - prevMS)
			if delta > 0 {
				time.Sleep(time.Duration(float64(delta) * float64(time.Millisecond) / r.Speed))
			}
			prevMS = int64(frame.Millis)
		}

		BroadcastParsedSensorData(h, uint64(frame.DID), frame.Data, int(time.Now().UnixMilli()))

		frameIndex++
	}
}
