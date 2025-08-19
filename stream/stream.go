package stream

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
	colours []string
	// points holds the actual data. Note that it will only ever hold data that fits into the current window.
	points []*DataPoint
}

func NewStream(
	key,
	description,
	unit string,
	discrete bool,
	smoothingAlpha float64,
	precision uint8,
	colours []string,
) *Stream {
	return &Stream{
		key,
		description,
		unit,
		discrete,
		smoothingAlpha,
		precision,
		colours,
		make([]*DataPoint, 0),
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

func (s *Stream) Colours() []string {
	return s.colours
}

func (s *Stream) Points() []*DataPoint {
	return s.points
}

func (s *Stream) Add(timestamp int, value float64) {
	point := &DataPoint{
		timestamp,
		value,
	}
	s.points = append(s.points, point)
}

func (s *Stream) Latest() *DataPoint {
	if len(s.points) == 0 {
		return &DataPoint{0, 0}
	}
	return s.points[len(s.points)-1]
}
