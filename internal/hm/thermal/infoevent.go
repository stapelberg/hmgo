package thermal

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stapelberg/homematic/internal/hm"
)

var (
	infoEventSetTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventSetTemperature",
			Help:      "target temperature in degC",
		},
		[]string{"address", "name"})

	infoEventActualTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventActualTemperature",
			Help:      "current temperature in degC",
		},
		[]string{"address", "name"})

	infoEventLowbatReporting = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventLowbatReporting",
			Help:      "low battery as bool",
		},
		[]string{"address", "name"})

	infoEventCommunicationReporting = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventCommunicationReporting",
			Help:      "communication reporting as bool",
		},
		[]string{"address", "name"})

	infoEventWindowOpenReporting = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventWindowOpenReporting",
			Help:      "window open reporting as bool",
		},
		[]string{"address", "name"})

	infoEventBatteryState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventBatteryState",
			Help:      "battery state in V",
		},
		[]string{"address", "name"})

	infoEventControl = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventControl",
			Help:      "control mode",
		},
		[]string{"address", "name"})

	infoEventBoostState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventBoostState",
			Help:      "boost state",
		},
		[]string{"address", "name"})
)

func init() {
	prometheus.MustRegister(infoEventSetTemperature)
	prometheus.MustRegister(infoEventActualTemperature)
	prometheus.MustRegister(infoEventLowbatReporting)
	prometheus.MustRegister(infoEventCommunicationReporting)
	prometheus.MustRegister(infoEventWindowOpenReporting)
	prometheus.MustRegister(infoEventBatteryState)
	prometheus.MustRegister(infoEventControl)
	prometheus.MustRegister(infoEventBoostState)
}

type ControlMode uint

const (
	AutoMode ControlMode = iota
	ManuMode
	PartyMode
	BoostMode
)

func (cm ControlMode) String() string {
	switch cm {
	case AutoMode:
		return "Auto"
	case ManuMode:
		return "Manu"
	case PartyMode:
		return "Party"
	case BoostMode:
		return "Boost"
	default:
		return fmt.Sprintf("unknown mode (%d dec, %x hex)", uint(cm), uint(cm))
	}
}

type InfoEvent struct {
	SetTemperature         float64 // in degC
	ActualTemperature      float64 // in degC
	LowbatReporting        bool
	CommunicationReporting bool
	WindowOpenReporting    bool
	BatteryState           float64 // in V
	Control                ControlMode
	BoostState             uint64
	// Not using party mode, so ignoring the remaining fields.
}

var ieTmpl = template.Must(template.New("infoevent").Parse(`
<strong>Info:</strong><br>
Target temperature: {{ .SetTemperature }} ℃<br>
Current temperature: {{ .ActualTemperature }} ℃<br>
Low battery: {{ .LowbatReporting }}<br>
TODO: Communication: {{ .CommunicationReporting }}<br>
Window open: {{ .WindowOpenReporting }}<br>
Battery state: {{ .BatteryState }} V<br>
Control: {{ .Control }}<br>
Boost state: {{ .BoostState }}<br>
`))

func (ie *InfoEvent) HTML() template.HTML {
	var buf bytes.Buffer
	if err := ieTmpl.Execute(&buf, ie); err != nil {
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return template.HTML(buf.String())
}

func (tc *ThermalControl) DecodeInfoEvent(payload []byte) (*InfoEvent, error) {
	// c.f. <frame id="INFO_LEVEL"> in rftypes/tc.xml
	if got, want := len(payload), 5; got != want {
		return nil, fmt.Errorf("unexpected payload size: got %d, want %d", got, want)
	}
	ie := &InfoEvent{
		SetTemperature:         float64((uint64(payload[1])>>2)&hm.Mask6Bit) / 2,
		ActualTemperature:      float64((int64(payload[1])&hm.Mask2Bit)|int64(payload[2])) / 10,
		LowbatReporting:        (payload[3] >> 7) == 1,
		CommunicationReporting: (payload[3] >> 6) == 1,
		WindowOpenReporting:    (payload[3] >> 5) == 1,
		BatteryState:           float64(payload[3]&hm.Mask5Bit)/10 + 1.5,
		Control:                ControlMode((payload[4] >> 6) & hm.Mask2Bit),
		BoostState:             uint64(payload[4] & hm.Mask6Bit),
	}

	infoEventSetTemperature.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(ie.SetTemperature)
	infoEventActualTemperature.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(ie.ActualTemperature)
	infoEventLowbatReporting.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(boolToFloat64(ie.LowbatReporting))
	infoEventCommunicationReporting.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(boolToFloat64(ie.CommunicationReporting))
	infoEventWindowOpenReporting.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(boolToFloat64(ie.WindowOpenReporting))
	infoEventBatteryState.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(ie.BatteryState)
	infoEventControl.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(float64(ie.Control))
	infoEventBoostState.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(float64(ie.BoostState))

	tc.latestMu.Lock()
	defer tc.latestMu.Unlock()
	tc.latestInfoEvent = ie
	return ie, nil
}

func boolToFloat64(val bool) float64 {
	var converted float64
	if val {
		converted = 1
	}
	return converted
}
