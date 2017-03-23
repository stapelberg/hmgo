package heating

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stapelberg/hmgo/internal/hm"
)

const prometheusNamespace = "hmheating"

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

	infoEventFault = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventFault",
			Help:      "fault as bool",
		},
		[]string{"address", "name", "fault"})

	infoEventBatteryState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventBatteryState",
			Help:      "battery state in V",
		},
		[]string{"address", "name"})

	infoEventValveState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventValveState",
			Help:      "valve state in percentage points",
		},
		[]string{"address", "name"})

	infoEventControl = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventControl",
			Help:      "control mode",
		},
		[]string{"address", "name", "mode"})

	infoEventBoostState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "InfoEventBoostState",
			Help:      "boost state in minutes",
		},
		[]string{"address", "name"})
)

func init() {
	prometheus.MustRegister(infoEventSetTemperature)
	prometheus.MustRegister(infoEventActualTemperature)
	prometheus.MustRegister(infoEventFault)
	prometheus.MustRegister(infoEventBatteryState)
	prometheus.MustRegister(infoEventValveState)
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

type FaultReporting uint

const (
	NoFault FaultReporting = iota
	ValveTight
	AdjustingRangeTooLarge
	AdjustingRangeTooSmall
	CommunicationError
	_
	Lowbat
	ValveErrorPosition
)

func (fr FaultReporting) String() string {
	switch fr {
	case NoFault:
		return "none"
	case ValveTight:
		return "valve tight"
	case AdjustingRangeTooLarge:
		return "adjusting range too large"
	case AdjustingRangeTooSmall:
		return "adjusting range too small"
	case CommunicationError:
		return "communication error"
	case Lowbat:
		return "low battery"
	case ValveErrorPosition:
		return "valve error position"
	default:
		return fmt.Sprintf("unknown fault (%d dec, %x hex)", uint(fr), uint(fr))
	}
}

type InfoEvent struct {
	SetTemperature    float64 // in degC
	ActualTemperature float64 // in degC
	Fault             FaultReporting
	BatteryState      float64 // in V
	ValveState        uint64  // in percentage points
	Control           ControlMode
	BoostState        uint64 // in minutes
	// Not using party mode, so ignoring the remaining fields.
}

var ieTmpl = template.Must(template.New("infoevent").Parse(`
<strong>Info:</strong><br>
Target temperature: {{ .SetTemperature }} ℃<br>
Current temperature: {{ .ActualTemperature }} ℃<br>
Fault: {{ .Fault }}<br>
Battery state: {{ .BatteryState }} V<br>
Valve state: {{ .ValveState }}%<br>
Control: {{ .Control }}<br>
Boost state: {{ .BoostState }} minutes<br>
`))

func (ie *InfoEvent) HTML() template.HTML {
	var buf bytes.Buffer
	if err := ieTmpl.Execute(&buf, ie); err != nil {
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return template.HTML(buf.String())
}

func (t *Thermostat) DecodeInfoEvent(payload []byte) (*InfoEvent, error) {
	// c.f. <frame id="INFO_LEVEL"> in rftypes/cc.xml
	if got, want := len(payload), 6; got != want {
		return nil, fmt.Errorf("unexpected payload size: got %d, want %d", got, want)
	}
	ie := &InfoEvent{
		SetTemperature:    float64((uint64(payload[1])>>2)&hm.Mask6Bit) / 2,
		ActualTemperature: float64((int64(payload[1])&hm.Mask2Bit)|int64(payload[2])) / 10,
		Fault:             FaultReporting((payload[3] >> 5) & hm.Mask3Bit),
		BatteryState:      float64(payload[3]&hm.Mask5Bit)/10 + 1.5,
		ValveState:        uint64(payload[4] & hm.Mask7Bit),
		Control:           ControlMode((payload[5] >> 6) & hm.Mask2Bit),
		BoostState:        uint64(payload[5] & hm.Mask6Bit),
	}

	infoEventSetTemperature.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex()}).Set(ie.SetTemperature)
	infoEventActualTemperature.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex()}).Set(ie.ActualTemperature)

	infoEventFault.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "fault": "valvetight"}).Set(boolToFloat64(ie.Fault == ValveTight))
	infoEventFault.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "fault": "adjustingrangetoosmall"}).Set(boolToFloat64(ie.Fault == AdjustingRangeTooSmall))
	infoEventFault.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "fault": "adjustingrangetoolarge"}).Set(boolToFloat64(ie.Fault == AdjustingRangeTooLarge))
	infoEventFault.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "fault": "communicationerror"}).Set(boolToFloat64(ie.Fault == CommunicationError))
	infoEventFault.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "fault": "lowbat"}).Set(boolToFloat64(ie.Fault == Lowbat))
	infoEventFault.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "fault": "valveerrorposition"}).Set(boolToFloat64(ie.Fault == ValveErrorPosition))

	infoEventBatteryState.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex()}).Set(ie.BatteryState)
	infoEventValveState.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex()}).Set(float64(ie.ValveState))

	infoEventControl.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "mode": "manu"}).Set(boolToFloat64(ie.Control == ManuMode))
	infoEventControl.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "mode": "party"}).Set(boolToFloat64(ie.Control == PartyMode))
	infoEventControl.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex(), "mode": "boost"}).Set(boolToFloat64(ie.Control == BoostMode))

	infoEventBoostState.With(prometheus.Labels{"name": t.Name(), "address": t.AddrHex()}).Set(float64(ie.BoostState))

	t.latestMu.Lock()
	defer t.latestMu.Unlock()
	t.latestInfoEvent = ie
	return ie, nil
}

func boolToFloat64(val bool) float64 {
	var converted float64
	if val {
		converted = 1
	}
	return converted
}
