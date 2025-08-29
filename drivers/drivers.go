package drivers

import (
	"time"

	"huskki/ecus"
	"huskki/models"
	"huskki/store"
)

const (
	LOG_DIR              = "logs"
	LOG_NAME             = "RAWLOG"
	LOG_EXT              = ".bin"
	WRITE_EVERY_N_FRAMES = 100
)

type Driver interface {
	Init() error
	Run() error
}

func addDidDataToStream(didData []*ecus.DIDData) {
	for _, didDatum := range didData {
		if didDatum.StreamKey != "" {
			if stream, ok := store.DashboardStreams[didDatum.StreamKey]; ok {
				addPointToStream(stream, didDatum)
			}
		}
	}
}

func addPointToStream(stream *models.Stream, didDatum *ecus.DIDData) {
	if stream.Discrete() {
		// Add point with same timestamp and the last point's value if this is discrete data so we get that nice
		// stepped look
		// Set time back 1 ms so we don't have multiple points on the same timestamp
		stream.Add(int(time.Now().UnixMilli())-1, stream.Latest().Value())
	}

	stream.Add(int(time.Now().UnixMilli()), didDatum.DidValue)
}
