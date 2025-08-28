package models

type ColourStop struct {
	Offset string // e.g. "0%", "50%", "100%"
	Color  string // e.g. "#ff0000"
}

type Stream struct {
	// key is the identifier and doubles as the name (probably a terrible idea, but it hasn't bitten me yet).
	key string
	// description is just some more info about this stream.
	description string
	// unit of the values in points.
	unit string
	// discrete determines whether to treat data points as discrete which essentially means when they change there is no
	// "in between" values. An example is gears; selected gear could be 1 or 2, but can't be 1.5.
	discrete bool
	// colours is an array of colours to use if this data is displayed, it is treated as a gradient where low values
	// are given the first colour in the slice, and high values are given the last colour in the slice. Colours should
	// be specified as 3 byte hex with the # prefix.
	colours []ColourStop
	// min is the minimum value to show on the y-axis
	min float64
	// max is the maximum value to show on the y-axis
	max float64
	// windowSize determines how many milliseconds worth of data to show.
	windowSize int
	// IsActive determines whether this stream is the active stream within it's chart
	IsActive bool
	// svgPoints holds recent point data that hasn't been sent to clients, post processed for display as an SVG sparkline.
	svgPoints []DataPoint
	// currentTimeMs is the current time in ms
	// TODO: this could be replaced with a more central timer passed through in tick, was just lazy
	currentTimeMs int
	// startTimeMs is the timestamp of the first point in the stream
	startTimeMs int
	// latest is the last point sent to the stream
	latest DataPoint
}

func NewStream(
	key,
	description,
	unit string,
	discrete bool,
	colours []ColourStop,
	min float64,
	max float64,
	windowSize int,
	isActive bool,
) *Stream {
	return &Stream{
		key,
		description,
		unit,
		discrete,
		colours,
		min,
		max,
		windowSize,
		isActive,
		make([]DataPoint, 0),
		0,
		0,
		DataPoint{},
	}
}

func (s *Stream) Key() string {
	return s.key
}

func (s *Stream) Description() string {
	return s.description
}

func (s *Stream) Unit() string {
	return s.unit
}

func (s *Stream) Discrete() bool {
	return s.discrete
}

func (s *Stream) Colours() []ColourStop {
	return s.colours
}

func (s *Stream) Max() float64 {
	return s.max
}

func (s *Stream) Min() float64 {
	return s.min
}

func (s *Stream) WindowSize() int {
	return s.windowSize
}

func (s *Stream) SvgPoints() []DataPoint {
	return s.svgPoints
}

func (s *Stream) Add(timestamp int, value float64) {
	s.latest = DataPoint{
		timestamp: timestamp,
		value:     value,
	}

	// Generate and append the svg point
	svgPoint := DataPoint{
		timestamp + s.windowSize - s.StartTimeMs(),
		s.max + s.min - value,
	}
	s.svgPoints = append(s.svgPoints, svgPoint)
}

func (s *Stream) Latest() DataPoint {
	return s.latest
}

func (s *Stream) CurrentTimeMs() int {
	return s.currentTimeMs
}

func (s *Stream) LeftX() int {
	if s.StartTimeMs() == 0 {
		return 0
	}
	return s.currentTimeMs - s.StartTimeMs()
}

func (s *Stream) RightX() int {
	if s.StartTimeMs() == 0 {
		return 0
	}
	return s.currentTimeMs - s.StartTimeMs() + s.windowSize
}

func (s *Stream) StartTimeMs() int {
	if s.startTimeMs == 0 {
		s.startTimeMs = s.currentTimeMs
	}
	return s.startTimeMs
}

func (s *Stream) OnTick(currentTimeMs int) {
	s.currentTimeMs = currentTimeMs
	// Clear points array because we have sent it to the frontend
	s.svgPoints = s.svgPoints[:0]
}
