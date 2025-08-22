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
	// smoothingAlpha determines how much smoothing to apply, 1 is no smoothing and very responsive, 0 is lots of
	// smoothing and less responsive. This uses EMA smoothing.
	smoothingAlpha float64
	// precision determines how many decimal places to show when displaying the value.
	precision uint8
	// colours is an array of colours to use if this data is displayed, it is treated as a gradient where low values
	// are given the first colour in the slice, and high values are given the last colour in the slice. Colours should
	// be specified as 3 byte hex without the #.
	colours []ColourStop
	// min is the minimum value to show on the y-axis
	min float64
	// max is the maximum value to show on the y-axis
	max float64
	// windowSize determines how many milliseconds worth of data to show.
	windowSize int
	// IsActive determines whether this stream is the active stream within it's chart
	IsActive bool
	// points holds the actual data.
	points []DataPoint
	// window holds point data, post processed from points.
	window []DataPoint
}

func NewStream(
	key,
	description,
	unit string,
	discrete bool,
	smoothingAlpha float64,
	precision uint8,
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
		smoothingAlpha,
		precision,
		colours,
		min,
		max,
		windowSize,
		isActive,
		make([]DataPoint, 0),
		make([]DataPoint, 0),
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

func (s *Stream) SmoothingAlpha() float64 {
	return s.smoothingAlpha
}

func (s *Stream) Precision() uint8 {
	return s.precision
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

func (s *Stream) Window() []DataPoint {
	return s.window
}

func (s *Stream) Add(timestamp int, value float64) {
	point := DataPoint{
		timestamp,
		value,
	}
	s.points = append(s.points, point)
}

func (s *Stream) Latest() DataPoint {
	if len(s.points) == 0 {
		return DataPoint{0, 0}
	}
	return s.points[len(s.points)-1]
}

func (s *Stream) OnTick(currentTimeMs int) {
	s.PostProcess(currentTimeMs)
}

func (s *Stream) PostProcess(currentTimeMs int) {
	// Re-use the backing array and capacity; slightly more performant than nuking the whole array.
	s.window = s.window[:0]

	if len(s.points) == 0 {
		return
	}

	leftMs := currentTimeMs - s.windowSize

	// Find first index >= leftMs (start of window)
	start := 0
	for i, p := range s.points {
		if p.timestamp >= leftMs {
			start = i
			break
		}
		// if all points < leftMs, set to len so that when we subtract the 1 point margin we grab the last point.
		start = i + 1
	}
	// Add 1-point margin to the left if possible
	if start > 0 {
		start-- // include the last point before the window
	}

	// Find last index <= currentTimeMs (end of window)
	end := len(s.points) - 1
	for i := start; i < len(s.points); i++ {
		if s.points[i].timestamp > currentTimeMs {
			end = i - 1
			break
		}
	}
	if end < start {
		end = start // window degenerate, keep at least one point
	}

	// Add 1-point margin to the right if possible
	if end+1 < len(s.points) {
		end++
	}

	// Slice is [start, end] inclusive → make endExclusive = end+1
	endExclusive := end + 1
	if endExclusive > len(s.points) {
		endExclusive = len(s.points)
	}

	// Extract our window of data, ensuring to copy so we don't mutate the original array as src is actually just a
	// slice header to the same points slice
	src := s.points[start:endExclusive]
	s.window = append([]DataPoint(nil), src...)

	if len(s.window) == 0 {
		return
	}

	// Normalise time from 0 to windowSize
	for i := 0; i < len(s.window); i++ {
		s.window[i].timestamp += s.windowSize - currentTimeMs
	}

	// Add sentinel that follows last point's value
	sentinel := DataPoint{
		s.windowSize,
		s.window[len(s.window)-1].value,
	}

	s.window = append(s.window, sentinel)

	// Invert value as SVG Y is flipped
	for i := 0; i < len(s.window); i++ {
		s.window[i].value = s.max + s.min - s.window[i].value
	}
}
