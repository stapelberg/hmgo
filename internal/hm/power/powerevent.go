package power

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stapelberg/homematic/internal/hm"
)

const prometheusNamespace = "hmpower"

var (
	powerEventBoot = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "PowerEventBoot",
			Help:      "booted state (bool)",
		},
		[]string{"address", "name"})

	powerEventEnergyCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "PowerEventEnergyCounter",
			Help:      "energy counter in Wh",
		},
		[]string{"address", "name"})

	powerEventPower = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "PowerEventPower",
			Help:      "power in W",
		},
		[]string{"address", "name"})

	powerEventCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "PowerEventCurrent",
			Help:      "current in mA",
		},
		[]string{"address", "name"})

	powerEventVoltage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "PowerEventVoltage",
			Help:      "voltage in V",
		},
		[]string{"address", "name"})

	powerEventFrequency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "PowerEventFrequency",
			Help:      "frequency in Hz",
		},
		[]string{"address", "name"})
)

func init() {
	prometheus.MustRegister(powerEventBoot)
	prometheus.MustRegister(powerEventEnergyCounter)
	prometheus.MustRegister(powerEventPower)
	prometheus.MustRegister(powerEventCurrent)
	prometheus.MustRegister(powerEventVoltage)
	prometheus.MustRegister(powerEventFrequency)
}

type PowerEvent struct {
	Boot          bool
	EnergyCounter float64 // in Wh
	Power         float64 // in W
	Current       float64 // in mA
	Voltage       float64 // in V
	Frequency     float64 // in Hz
}

var peTmpl = template.Must(template.New("powerevent").Parse(`
<strong>Power:</strong><br>
Booted: {{ .Boot }}<br>
EnergyCounter: {{ .EnergyCounter }} Wh<br>
Power: {{ .Power }} W<br>
Current {{ .Current }} mA<br>
Voltage: {{ .Voltage }} V<br>
Frequency: {{ .Frequency }} Hz<br>
`))

func (pe *PowerEvent) HTML() template.HTML {
	var buf bytes.Buffer
	if err := peTmpl.Execute(&buf, pe); err != nil {
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return template.HTML(buf.String())
}

func (ps *PowerSwitch) DecodePowerEvent(payload []byte) (*PowerEvent, error) {
	// c.f. <frame id="POWER_EVENT"> in rftypes/es2.xml
	if got, want := len(payload), 11; got != want {
		return nil, fmt.Errorf("unexpected payload size: got %d, want %d", got, want)
	}
	pe := &PowerEvent{
		Boot:          ((payload[0] >> 7) & hm.Mask1Bit) == 1,
		EnergyCounter: float64(((uint64(payload[0])&hm.Mask7Bit)<<16)|(uint64(payload[1])<<8)|uint64(payload[2])) / 10,
		Power:         float64((uint64(payload[3])<<16)|(uint64(payload[4])<<8)|uint64(payload[5])) / 100,
		Current:       float64((uint64(payload[6]) << 8) | uint64(payload[7])),
		Voltage:       float64(uint64(payload[8])<<8|uint64(payload[9])) / 10,
		Frequency:     float64(int64(payload[10]))/100 + 50,
	}

	var boot float64
	if pe.Boot {
		boot = 1
	}
	powerEventBoot.With(prometheus.Labels{"name": ps.Name(), "address": ps.AddrHex()}).Set(boot)
	powerEventEnergyCounter.With(prometheus.Labels{"name": ps.Name(), "address": ps.AddrHex()}).Set(pe.EnergyCounter)
	powerEventPower.With(prometheus.Labels{"name": ps.Name(), "address": ps.AddrHex()}).Set(pe.Power)
	powerEventCurrent.With(prometheus.Labels{"name": ps.Name(), "address": ps.AddrHex()}).Set(pe.Current)
	powerEventVoltage.With(prometheus.Labels{"name": ps.Name(), "address": ps.AddrHex()}).Set(pe.Voltage)
	powerEventFrequency.With(prometheus.Labels{"name": ps.Name(), "address": ps.AddrHex()}).Set(pe.Frequency)

	ps.latestMu.Lock()
	defer ps.latestMu.Unlock()
	ps.latestPowerEvent = pe
	return pe, nil
}
