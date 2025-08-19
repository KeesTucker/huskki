package ui_components

import "huskki/stream"

type Chart struct {
	// key is the identifier and doubles as the name (probably a terrible idea, but it hasn't bitten me yet).
	key string
	// ActiveStream is the index of the currently selected stream in this chart.
	ActiveStream uint8
	// max is the maximum value to show on the y-axis
	max float64
	// min is the minimum value to show on the y-axis
	min float64
	// windowSize determines how many milliseconds worth of data to show.
	windowSize int

	// streams to display in this chart
	streams []*stream.Stream
}

func NewChart(
	key string,
	max,
	min float64,
	windowSize int,
	streams []*stream.Stream,
) *Chart {
	return &Chart{
		key,
		0,
		max,
		min,
		windowSize,
		streams,
	}
}

func (c *Chart) Key() string {
	return c.key
}

func (c *Chart) Max() float64 {
	return c.max
}

func (c *Chart) Min() float64 {
	return c.max
}

func (c *Chart) WindowSize() int {
	return c.windowSize
}

func (c *Chart) Streams() []*stream.Stream {
	return c.streams
}
