package ecus

type DIDData struct {
	StreamKey string
	DidValue  float64
}

type ECUProcessor interface {
	ParseDIDBytes(did uint32, dataBytes []byte) []*DIDData
}
