package drivers

// Frame is one validated can bus frame from the stream/log.
type Frame struct {
	Millis uint32 // LE from hdr[0..3]
	DID    uint16 // BE from hdr[4..5]
	Data   []byte // len = hdr[6]
}

type Driver interface {
	Init() error
	Run() error
}
