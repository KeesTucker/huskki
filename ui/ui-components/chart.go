package ui_components

import "huskki/stream"

type Chart struct {
	// key is the identifier and doubles as the name (probably a terrible idea, but it hasn't bitten me yet).
	key string
	// streams to display in this chart
	streams []*stream.Stream
}

func NewChart(
	key string,
	streams []*stream.Stream,
) *Chart {
	return &Chart{
		key,
		streams,
	}
}

func (c *Chart) Key() string {
	return c.key
}

func (c *Chart) Streams() []*stream.Stream {
	return c.streams
}
