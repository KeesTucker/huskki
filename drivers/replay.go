package drivers

import (
	"bufio"
	"errors"
	"huskki/config"
	"huskki/ecus"
	"huskki/store"
	"io"
	"log"
	"os"
	"time"
)

type Replayer struct {
	*config.ReplayFlags
	ecuProcessor ecus.ECUProcessor
}

func NewReplayer(replayFlags *config.ReplayFlags, processor ecus.ECUProcessor) *Replayer {
	replayer := &Replayer{
		replayFlags,
		processor,
	}
	return replayer
}

func (r *Replayer) Run() error {
	for {
		if err := r.playOnce(); err != nil {
			return err
		}
		if !r.Loop {
			break
		}
	}
	return nil
}

func (r *Replayer) Init() error {
	return nil
}

func (r *Replayer) playOnce() error {
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
		did, value, timestamp, err := readBinaryFrame(bufferReader)
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
			prevMS = int64(timestamp)
		}

		if r.Speed > 0 {
			delta := time.Duration(int64(timestamp) - prevMS)
			if delta > 0 {
				time.Sleep(time.Duration(float64(delta) * float64(time.Millisecond) / r.Speed))
			}
			prevMS = int64(timestamp)
		}

		key, didValue := r.ecuProcessor.ParseDIDBytes(uint64(did), value)
		if key != "" {
			stream, ok := store.DashboardStreams[key]
			if ok {
				if stream.Discrete() {
					// Add point with same timestamp and the last point's value if this is discrete data so we get that nice
					// stepped look
					stream.Add(int(time.Now().UnixMilli()), didValue)
				}

				stream.Add(int(time.Now().UnixMilli()), didValue)
			}
		}

		frameIndex++
	}
}
