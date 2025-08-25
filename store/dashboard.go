package store

import (
	"huskki/models"
	"maps"
	"slices"
	"sort"
)

const DASHBOARD_FRAMERATE = 30

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

var DashboardStreams = map[string]*models.Stream{
	THROTTLE_STREAM: models.NewStream(
		THROTTLE_STREAM,
		"ECU computed throttle",
		"%",
		false,
		0.5,
		1,
		[]models.ColourStop{
			{"100%", "#002550"},
		},
		-5,
		105,
		10000,
		false,
	),
	GRIP_STREAM: models.NewStream(
		GRIP_STREAM,
		"Rider throttle input",
		"%",
		false,
		0.5,
		1,
		[]models.ColourStop{
			{"100%", "#FFED00"},
		},
		-5,
		105,
		10000,
		true,
	),
	TPS_STREAM: models.NewStream(
		TPS_STREAM,
		"Throttle plate sensor",
		"%",
		false,
		0.5,
		1,
		[]models.ColourStop{
			{"100%", "#ffffff"},
		},
		-5,
		105,
		10000,
		false,
	),
	RPM_STREAM: models.NewStream(
		RPM_STREAM,
		"Engine rotational speed",
		"rpm",
		false,
		0.5,
		0,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0,
		10000,
		10000,
		true,
	),
	GEAR_STREAM: models.NewStream(
		GEAR_STREAM,
		"Transmission Gear",
		"",
		true,
		0.5,
		0,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		-1,
		7,
		10000,
		true,
	),
	COOLANT_STREAM: models.NewStream(
		COOLANT_STREAM,
		"Coolant temperature",
		"°C",
		false,
		0.5,
		1,
		[]models.ColourStop{
			{"0%", "#FF0000"},
			{"50%", "#00FF00"},
			{"100%", "#0000FF"},
		},
		-10,
		120,
		300000,
		true,
	),
	INJECTION_TIME_STREAM: models.NewStream(
		INJECTION_TIME_STREAM,
		"Injector pulse width",
		"ms",
		false,
		0.5,
		2,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0,
		15,
		10000,
		true,
	),
}

var DashboardCharts = map[string]*models.Chart{
	THROTTLE_CHART: models.NewChart(
		THROTTLE_CHART,
		[]*models.Stream{DashboardStreams[THROTTLE_STREAM], DashboardStreams[GRIP_STREAM], DashboardStreams[TPS_STREAM]},
		1,
	),
	RPM_CHART: models.NewChart(
		RPM_CHART,
		[]*models.Stream{DashboardStreams[RPM_STREAM]},
		2,
	),
	GEAR_CHART: models.NewChart(
		GEAR_CHART,
		[]*models.Stream{DashboardStreams[GEAR_STREAM]},
		3,
	),
	COOLANT_CHART: models.NewChart(
		COOLANT_CHART,
		[]*models.Stream{DashboardStreams[COOLANT_STREAM]},
		4,
	),
	INJECTION_CHART: models.NewChart(
		INJECTION_CHART,
		[]*models.Stream{DashboardStreams[INJECTION_TIME_STREAM]},
		5,
	),
}

var orderedCharts []*models.Chart

func OrderedCharts() []*models.Chart {
	if orderedCharts == nil {
		orderedCharts = slices.Collect(maps.Values(DashboardCharts))
		sort.Slice(orderedCharts, func(i, j int) bool {
			return orderedCharts[i].LayoutPriority() < orderedCharts[j].LayoutPriority()
		})
	}
	return orderedCharts
}
