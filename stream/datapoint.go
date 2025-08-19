package stream

type DataPoint struct {
	timestamp int
	value     float64
}

func (p *DataPoint) Timestamp() int {
	return p.timestamp
}

func (p *DataPoint) Value() float64 {
	return p.value
}
