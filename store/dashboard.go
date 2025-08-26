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
	FRONT_BRAKE_STREAM      = "Front-Brake"
	SIDESTAND_STREAM        = "Side-Stand"
	SAS_VALVE_STREAM        = "SAS-Valve"
	CYL1_O2_VOLT_STREAM     = "O2-Voltage"
	CYL1_O2_COMP_STREAM     = "O2-Compensation"
	IAP_VOLTAGE_STREAM      = "IAP-Voltage"
	CYL1_O2_ADC_STREAM      = "O2-Adc"
	CYL1_O2_EXTENDED_STREAM = "O2-Extended"
	IAP_STREAM              = "IAP"
	CYL1_COIL1_STREAM       = "Coil-1-Current"
	CYL1_COIL2_STREAM       = "Coil-2-Current"
	CYL1_COIL1_DWELL_STREAM = "Coil-1-Dwell"
	CYL1_COIL2_DWELL_STREAM = "Coil-2-Dwell"
	ENGINE_LOAD_STREAM      = "Engine-Load"
	BARO_VOLT_STREAM        = "Barometer-Volt"
	BARO_STREAM             = "Estimated-Altitude"
)

const (
	THROTTLE_CHART  = "Throttle"
	RPM_CHART       = "RPM"
	COOLANT_CHART   = "Coolant"
	INJECTION_CHART = "Injection"
	SWITCHES_CHART  = "Switches"
	CYL1_O2_CHART   = "O2"
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
		"",
		true,
		[]models.ColourStop{
			{"0%", "#777777"},
			{"100%", "#00D084"},
		},
		-0.2, 1.2, 10000, false,
	),
	FRONT_BRAKE_STREAM: models.NewStream(
		FRONT_BRAKE_STREAM,
		"Front brake pressure",
		"%",
		false,
		[]models.ColourStop{
			{"0%", "#777777"},
			{"100%", "#00D084"},
		},
		-20, 120, 10000, false,
	),
	SAS_VALVE_STREAM: models.NewStream(
		SAS_VALVE_STREAM,
		"Sas valve opening",
		"",
		true,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		-0.2, 1.2, 10000, false,
	),
	CYL1_O2_VOLT_STREAM: models.NewStream(
		CYL1_O2_VOLT_STREAM,
		"O₂ sensor voltage",
		"V",
		false,
		[]models.ColourStop{
			{"0%", "#0033FF"},
			{"100%", "#66CCFF"},
		},
		-0.2, 1.2, 10000, true,
	),
	CYL1_O2_COMP_STREAM: models.NewStream(
		CYL1_O2_COMP_STREAM,
		"Real Time Fuel Trim",
		"%",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		-0.5, 0.5, 10000, false,
	),
	CYL1_O2_ADC_STREAM: models.NewStream(
		CYL1_O2_ADC_STREAM,
		"O₂ ADC",
		"",
		false,
		[]models.ColourStop{
			{"0%", "#888888"},
			{"100%", "#DDDDDD"},
		},
		0, 1023, 10000, false,
	),
	CYL1_O2_EXTENDED_STREAM: models.NewStream(
		CYL1_O2_EXTENDED_STREAM,
		"O₂ extended",
		"V",
		false,
		[]models.ColourStop{
			{"0%", "#888888"},
			{"100%", "#DDDDDD"},
		},
		-0.2, 1.2, 10000, false,
	),
	IAP_VOLTAGE_STREAM: models.NewStream(
		IAP_VOLTAGE_STREAM,
		"Intake absolute pressure voltage",
		"",
		false,
		[]models.ColourStop{
			{"0%", "#888888"},
			{"100%", "#DDDDDD"},
		},
		0, 1023, 10000, false,
	),
	IAP_STREAM: models.NewStream(
		IAP_STREAM,
		"Intake absolute pressure",
		"",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 1023, 10000, true,
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
		0, 30, 10000, true,
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
		0, 30, 10000, false,
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
		0, 5, 10000, false,
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
		0, 5, 10000, false,
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
		0, 10,
		10000,
		false,
	),
	BARO_STREAM: models.NewStream(
		BARO_STREAM,
		"Estimated altitude",
		"m",
		false,
		[]models.ColourStop{
			{"0%", "#92FE9D"},
			{"100%", "#00C9FF"},
		},
		0, 2000,
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
		[]*models.Stream{DashboardStreams[GEAR_STREAM], DashboardStreams[CLUTCH_STREAM], DashboardStreams[FRONT_BRAKE_STREAM]},
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
	CYL1_O2_CHART: models.NewChart(
		CYL1_O2_CHART,
		[]*models.Stream{DashboardStreams[CYL1_O2_VOLT_STREAM], DashboardStreams[CYL1_O2_COMP_STREAM], DashboardStreams[CYL1_O2_ADC_STREAM], DashboardStreams[CYL1_O2_EXTENDED_STREAM]},
		6,
	),
	COIL_CHART: models.NewChart(
		COIL_CHART,
		[]*models.Stream{DashboardStreams[CYL1_COIL1_STREAM], DashboardStreams[CYL1_COIL2_STREAM], DashboardStreams[CYL1_COIL1_DWELL_STREAM], DashboardStreams[CYL1_COIL2_DWELL_STREAM]},
		7,
	),
	PRESSURE_CHART: models.NewChart(
		PRESSURE_CHART,
		[]*models.Stream{DashboardStreams[IAP_STREAM], DashboardStreams[IAP_VOLTAGE_STREAM], DashboardStreams[BARO_STREAM], DashboardStreams[BARO_VOLT_STREAM]},
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
