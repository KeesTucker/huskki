package config

import (
	"huskki/stream"
	"huskki/ui/ui-components"
)

const (
	THROTTLE_STREAM       = "Computed-Throttle"
	GRIP_STREAM           = "Input-Throttle"
	TPS_STREAM            = "TPS"
	RPM_STREAM            = "RPM"
	GEAR_STREAM           = "Gear"
	COOLANT_STREAM        = "Coolant"
	INJECTION_TIME_STREAM = "Injection-Time"
)

const (
	THROTTLE_CHART  = "Throttle"
	RPM_CHART       = "RPM"
	GEAR_CHART      = "Gear"
	COOLANT_CHART   = "Coolant"
	INJECTION_CHART = "Injection"
)

var dashboardStreams = map[string]*stream.Stream{
	THROTTLE_STREAM: stream.NewStream(
		THROTTLE_STREAM,
		"ECU computed throttle",
		"%",
		false,
		0.5,
		1,
		[]stream.ColourStop{
			{"0%", "#ff0000"},
			{"100%", "#ff00ff"},
		},
		-5,
		105,
		10000,
		false,
	),
	GRIP_STREAM: stream.NewStream(
		GRIP_STREAM,
		"Rider throttle input",
		"%",
		false,
		0.5,
		1,
		[]stream.ColourStop{
			{"0%", "#00ffff"},
			{"100%", "#0000ff"},
		},
		-5,
		105,
		10000,
		true,
	),
	TPS_STREAM: stream.NewStream(
		TPS_STREAM,
		"Throttle plate sensor",
		"%",
		false,
		0.5,
		1,
		[]stream.ColourStop{
			{"0%", "#00ff00"},
			{"100%", "#32cd32"},
		},
		-5,
		105,
		10000,
		false,
	),
	RPM_STREAM: stream.NewStream(
		RPM_STREAM,
		"Engine rotational speed",
		"rpm",
		false,
		0.5,
		0,
		[]stream.ColourStop{
			{"0%", "#cc0000"},
			{"25%", "#ba55d3"},
			{"50%", "#adff2f"},
			{"100%", "#d3d3d3"},
		},
		0,
		10000,
		10000,
		true,
	),
	GEAR_STREAM: stream.NewStream(
		GEAR_STREAM,
		"Transmission Gear",
		"",
		true,
		0.5,
		0,
		[]stream.ColourStop{
			{"0%", "#fa8bff"},
			{"50%", "#2bd2ff"},
			{"100%", "#2bff88"},
		},
		-1,
		7,
		10000,
		true,
	),
	COOLANT_STREAM: stream.NewStream(
		COOLANT_STREAM,
		"Coolant temperature",
		"°C",
		false,
		0.5,
		1,
		[]stream.ColourStop{
			{"0%", "#FF0000"},
			{"50%", "#00FF00"},
			{"100%", "#0000FF"},
		},
		-10,
		120,
		300000,
		true,
	),
	INJECTION_TIME_STREAM: stream.NewStream(
		INJECTION_TIME_STREAM,
		"Injector pulse width",
		"ms",
		false,
		0.5,
		2,
		[]stream.ColourStop{
			{"0%", "#f878ff"},
			{"50%", "##ffda9e"},
			{"100%", "#ffffff"},
		},
		0,
		15,
		10000,
		true,
	),
}

var dashboardCharts = map[string]*ui_components.Chart{
	THROTTLE_CHART: ui_components.NewChart(
		THROTTLE_CHART,
		[]*stream.Stream{dashboardStreams[THROTTLE_STREAM], dashboardStreams[GRIP_STREAM], dashboardStreams[TPS_STREAM]},
	),
	RPM_CHART: ui_components.NewChart(
		RPM_CHART,
		[]*stream.Stream{dashboardStreams[RPM_STREAM]},
	),
	GEAR_CHART: ui_components.NewChart(
		GEAR_CHART,
		[]*stream.Stream{dashboardStreams[GEAR_STREAM]},
	),
	COOLANT_CHART: ui_components.NewChart(
		COOLANT_CHART,
		[]*stream.Stream{dashboardStreams[COOLANT_STREAM]},
	),
	INJECTION_CHART: ui_components.NewChart(
		INJECTION_CHART,
		[]*stream.Stream{dashboardStreams[INJECTION_TIME_STREAM]},
	),
}
