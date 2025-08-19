package config

import (
	"huskki/stream"
	"huskki/ui/ui-components"
)

const (
	THROTTLE_STREAM       = "Computed Throttle"
	GRIP_STREAM           = "Input Throttle"
	TPS_STREAM            = "TPS"
	RPM_STREAM            = "RPM"
	GEAR_STREAM           = "Gear"
	COOLANT_STREAM        = "Coolant"
	INJECTION_TIME_STREAM = "Injection Time"
)

const (
	THROTTLE_CHART  = "Throttle"
	RPM_CHART       = "RPM"
	GEAR_CHART      = "Gear"
	COOLANT_CHART   = "Coolant"
	INJECTION_CHART = "Injection"
)

const WHITE = "ffffff"
const HUSQVARNA_BLUE = "002550"
const HUSQVARNA_YELLOW = "ffed00"

var dashboardStreams = map[string]*stream.Stream{
	THROTTLE_STREAM: stream.NewStream(
		THROTTLE_STREAM,
		"ECU computed throttle",
		"%",
		false,
		0.5,
		1,
		[]string{HUSQVARNA_YELLOW},
	),
	GRIP_STREAM: stream.NewStream(
		GRIP_STREAM,
		"Rider throttle input",
		"%",
		false,
		0.5,
		1,
		[]string{WHITE},
	),
	TPS_STREAM: stream.NewStream(
		TPS_STREAM,
		"Throttle plate sensor",
		"%",
		false,
		0.5,
		1,
		[]string{HUSQVARNA_BLUE},
	),
	RPM_STREAM: stream.NewStream(
		RPM_STREAM,
		"Engine rotational speed",
		"rpm",
		false,
		0.5,
		0,
		[]string{HUSQVARNA_YELLOW},
	),
	GEAR_STREAM: stream.NewStream(
		GEAR_STREAM,
		"Transmission Gear",
		"",
		true,
		0.5,
		0,
		[]string{WHITE},
	),
	COOLANT_STREAM: stream.NewStream(
		COOLANT_STREAM,
		"Coolant temperature",
		"°C",
		false,
		0.5,
		1,
		[]string{"c20ea1", "dd2d7f", "ee4c5e", "f46d41"},
	),
	INJECTION_TIME_STREAM: stream.NewStream(
		INJECTION_TIME_STREAM,
		"Injector pulse width",
		"ms",
		false,
		0.5,
		2,
		[]string{HUSQVARNA_YELLOW},
	),
}

var dashboardCharts = map[string]*ui_components.Chart{
	THROTTLE_CHART: ui_components.NewChart(
		THROTTLE_CHART,
		100,
		0,
		10000,
		[]*stream.Stream{dashboardStreams[THROTTLE_STREAM], dashboardStreams[GRIP_STREAM], dashboardStreams[TPS_STREAM]},
	),
	RPM_CHART: ui_components.NewChart(
		RPM_CHART,
		10000,
		0, 10000,
		[]*stream.Stream{dashboardStreams[RPM_STREAM]},
	),
	GEAR_CHART: ui_components.NewChart(
		GEAR_CHART,
		6,
		0,
		10000,
		[]*stream.Stream{dashboardStreams[GEAR_STREAM]},
	),
	COOLANT_CHART: ui_components.NewChart(
		COOLANT_CHART,
		120,
		0,
		300000,
		[]*stream.Stream{dashboardStreams[COOLANT_STREAM]},
	),
	INJECTION_CHART: ui_components.NewChart(
		INJECTION_CHART,
		10,
		0,
		10000,
		[]*stream.Stream{dashboardStreams[INJECTION_TIME_STREAM]},
	),
}
