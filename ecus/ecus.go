package ecus

type ECUProcessor interface {
	ParseDIDBytes(did uint64, dataBytes []byte) (key string, value float64)
}
