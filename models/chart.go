package models

type Chart struct {
	// key is the identifier and doubles as the name (probably a terrible idea, but it hasn't bitten me yet).
	key string
	// streams to display in this chart
	streams []*Stream
	// layoutPriority determines what order in the ui this chart should be shown
	layoutPriority uint8
}

func NewChart(
	key string,
	streams []*Stream,
	layoutPriority uint8,
) *Chart {
	return &Chart{
		key,
		streams,
		layoutPriority,
	}
}

func (c *Chart) Key() string {
	return c.key
}

func (c *Chart) Streams() []*Stream {
	return c.streams
}

func (c *Chart) LayoutPriority() uint8 {
	return c.layoutPriority
}
