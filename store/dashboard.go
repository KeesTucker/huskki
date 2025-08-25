package store

import (
	"huskki/models"
	"maps"
	"slices"
	"sort"
)

const DASHBOARD_FRAMERATE = 30

const (
	THROTTLE_STREAM         = "Computed-Throttle"
	GRIP_STREAM             = "Input-Throttle"
	TPS_STREAM              = "TPS"
	RPM_STREAM              = "RPM"
	GEAR_STREAM             = "Gear"
	COOLANT_STREAM          = "Coolant"
	INJECTION_TIME_STREAM   = "Injection-Time"
	CLUTCH_STREAM           = "Clutch"
	SIDESTAND_STREAM        = "Side-Stand"
	SWITCHES_MASK_STREAM    = "Switches-Mask"
	O2_REAR_VOLT_STREAM     = "O2-Rear-Voltage"
	O2_COMP_STREAM          = "O2-Comp"
	IAP_VOLT_STREAM         = "IAP-Voltage"
	IAP_STREAM              = "IAP"
	CYL1_COIL1_STREAM       = "Coil-1-Current"
	CYL1_COIL2_STREAM       = "Coil-2-Current"
	CYL1_COIL1_DWELL_STREAM = "Coil-1-Dwell"
	CYL1_COIL2_DWELL_STREAM = "Coil-2-Dwell"
	ENGINE_LOAD_STREAM      = "Engine-Load"
	BARO_VOLT_STREAM        = "Baro-Volt"
	BARO_STREAM             = "Baro"
)

const (
	THROTTLE_CHART  = "Throttle"
	RPM_CHART       = "RPM"
	COOLANT_CHART   = "Coolant"
	INJECTION_CHART = "Injection"
	SWITCHES_CHART  = "Switches"
	O2_CHART        = "O2"
	COIL_CHART      = "Coils"
	PRESSURE_CHART  = "Pressure"
)

var DashboardStreams = map[string]*models.Stream{
	THROTTLE_STREAM: models.NewStream(
		THROTTLE_STREAM,
		"ECU computed throttle",
		"%",
		false,
		[]models.ColourStop{
			{"100%", "#FF2200"},
		},
		-5, 105, 10000, false,
	),
	GRIP_STREAM: models.NewStream(
		GRIP_STREAM,
		"Rider throttle input",
		"%",
		false,
		[]models.ColourStop{
			{"100%", "#00FF22"},
		},
		-5, 105, 10000, true,
	),
	TPS_STREAM: models.NewStream(
		TPS_STREAM,
		"Throttle plate sensor",
		"%",
		false,
		[]models.ColourStop{
			{"100%", "#2200ff"},
		},
		-5, 105, 10000, false,
	),
	RPM_STREAM: models.NewStream(
		RPM_STREAM,
		"Engine rotational speed",
		"rpm",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 10000, 10000, true,
	),
	GEAR_STREAM: models.NewStream(
		GEAR_STREAM,
		"Transmission Gear",
		"",
		true,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		-1, 7, 10000, true,
	),
	COOLANT_STREAM: models.NewStream(
		COOLANT_STREAM,
		"Coolant temperature",
		"°C",
		false,
		[]models.ColourStop{
			{"0%", "#FF0000"},
			{"50%", "#00FF00"},
			{"100%", "#0000FF"},
		},
		-10, 120, 300000, true,
	),
	INJECTION_TIME_STREAM: models.NewStream(
		INJECTION_TIME_STREAM,
		"Injector pulse width",
		"ms",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 15, 10000, true,
	),
	CLUTCH_STREAM: models.NewStream(
		CLUTCH_STREAM,
		"Clutch switch",
		"", // 0 or 1
		true,
		[]models.ColourStop{
			{"0%", "#777777"},
			{"100%", "#00D084"},
		},
		0, 1, 10000, false,
	),
	SIDESTAND_STREAM: models.NewStream(
		SIDESTAND_STREAM,
		"Side stand",
		"", // 0 or 1
		true,
		[]models.ColourStop{
			{"0%", "#777777"},
			{"100%", "#FF6F00"},
		},
		0, 1, 10000, false,
	),
	SWITCHES_MASK_STREAM: models.NewStream(
		SWITCHES_MASK_STREAM,
		"Switches bitfield (low byte)",
		"mask", // 0..255
		true,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 255, 10000, false,
	),
	O2_REAR_VOLT_STREAM: models.NewStream(
		O2_REAR_VOLT_STREAM,
		"Rear O₂ sensor voltage",
		"V",
		false,
		[]models.ColourStop{
			{"0%", "#0033FF"},
			{"100%", "#66CCFF"},
		},
		0.0, 1.0, 10000, true,
	),
	O2_COMP_STREAM: models.NewStream(
		O2_COMP_STREAM,
		"O₂ compensation",
		"%",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 200, 10000, false,
	),
	IAP_VOLT_STREAM: models.NewStream(
		IAP_VOLT_STREAM,
		"IAP sensor voltage",
		"V",
		false,
		[]models.ColourStop{
			{"0%", "#888888"},
			{"100%", "#DDDDDD"},
		},
		0.0, 5.0, 10000, false,
	),
	IAP_STREAM: models.NewStream(
		IAP_STREAM,
		"Intake absolute pressure",
		"kPa",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 120, 10000, true,
	),
	CYL1_COIL1_STREAM: models.NewStream(
		CYL1_COIL1_STREAM,
		"Coil #1 primary current",
		"A",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 20, 10000, true,
	),
	CYL1_COIL2_STREAM: models.NewStream(
		CYL1_COIL2_STREAM,
		"Coil #2 primary current",
		"A",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 20, 10000, false,
	),
	CYL1_COIL1_DWELL_STREAM: models.NewStream(
		CYL1_COIL1_DWELL_STREAM,
		"Coil #1 dwell time",
		"ms",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 20, 10000, false,
	),
	CYL1_COIL2_DWELL_STREAM: models.NewStream(
		CYL1_COIL2_DWELL_STREAM,
		"Coil #2 dwell time",
		"ms",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 20, 10000, false,
	),
	ENGINE_LOAD_STREAM: models.NewStream(
		ENGINE_LOAD_STREAM,
		"Calculated engine load",
		"%",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 100,
		10000,
		false,
	),

	BARO_VOLT_STREAM: models.NewStream(
		BARO_VOLT_STREAM,
		"Atmospheric pressure sensor voltage",
		"V",
		false,
		[]models.ColourStop{
			{"0%", "#888888"},
			{"100%", "#DDDDDD"},
		},
		0.0, 5.0,
		10000,
		false,
	),

	BARO_STREAM: models.NewStream(
		BARO_STREAM,
		"Atmospheric pressure",
		"kPa",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		60, 110,
		10000,
		false,
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
		[]*models.Stream{DashboardStreams[RPM_STREAM], DashboardStreams[ENGINE_LOAD_STREAM]},
		2,
	),
	SWITCHES_CHART: models.NewChart(
		SWITCHES_CHART,
		[]*models.Stream{DashboardStreams[GEAR_STREAM], DashboardStreams[SWITCHES_MASK_STREAM], DashboardStreams[CLUTCH_STREAM], DashboardStreams[SIDESTAND_STREAM]},
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
	O2_CHART: models.NewChart(
		O2_CHART,
		[]*models.Stream{DashboardStreams[O2_REAR_VOLT_STREAM], DashboardStreams[O2_COMP_STREAM]},
		6,
	),
	COIL_CHART: models.NewChart(
		COIL_CHART,
		[]*models.Stream{DashboardStreams[CYL1_COIL1_STREAM], DashboardStreams[CYL1_COIL2_STREAM], DashboardStreams[CYL1_COIL1_DWELL_STREAM], DashboardStreams[CYL1_COIL2_DWELL_STREAM]},
		7,
	),
	PRESSURE_CHART: models.NewChart(
		PRESSURE_CHART,
		[]*models.Stream{DashboardStreams[IAP_STREAM], DashboardStreams[IAP_VOLT_STREAM], DashboardStreams[BARO_STREAM], DashboardStreams[BARO_VOLT_STREAM]},
		8,
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
