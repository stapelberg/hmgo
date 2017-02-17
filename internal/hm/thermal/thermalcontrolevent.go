package thermal

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stapelberg/homematic/internal/hm"
)

var (
	thermalControlEventSetTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "ThermalControlEventSetTemperature",
			Help:      "Temperature in degC",
		},
		[]string{"address", "name"})

	thermalControlEventActualTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "ThermalControlEventActualTemperature",
			Help:      "Temperature in degC",
		},
		[]string{"address", "name"})

	thermalControlEventActualHumidity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "ThermalControlEventActualHumidity",
			Help:      "Humidity in percentage points",
		},
		[]string{"address", "name"})
)

func init() {
	prometheus.MustRegister(thermalControlEventSetTemperature)
	prometheus.MustRegister(thermalControlEventActualTemperature)
	prometheus.MustRegister(thermalControlEventActualHumidity)
}

type ThermalControlEvent struct {
	SetTemperature    float64 // in degC
	ActualTemperature float64 // in degC
	ActualHumidity    float64 // in percentage points
}

var tceTmpl = template.Must(template.New("thermalcontrolevent").Parse(`
<strong>ThermalControl:</strong><br>
Target temperature: {{ .SetTemperature }} ℃<br>
Current temperature: {{ .ActualTemperature }} ℃<br>
Humidity: {{ .ActualHumidity }}%<br>
`))

func (tce *ThermalControlEvent) HTML() template.HTML {
	var buf bytes.Buffer
	if err := tceTmpl.Execute(&buf, tce); err != nil {
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return template.HTML(buf.String())
}

func (tc *ThermalControl) DecodeThermalControlEvent(payload []byte) (*ThermalControlEvent, error) {
	// c.f. <frame id="THERMALCONTROL_EVENT"> in rftypes/tc.xml
	if got, want := len(payload), 3; got != want {
		return nil, fmt.Errorf("unexpected payload size: got %d, want %d", got, want)
	}
	tce := &ThermalControlEvent{
		SetTemperature:    float64((uint64(payload[0])>>2)&hm.Mask6Bit) / 2,
		ActualTemperature: float64((int64(payload[0])&hm.Mask2Bit)|int64(payload[1])) / 10,
		ActualHumidity:    float64(payload[2]),
	}

	thermalControlEventSetTemperature.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(tce.SetTemperature)
	thermalControlEventActualTemperature.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(tce.ActualTemperature)
	thermalControlEventActualHumidity.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(float64(tce.ActualHumidity))

	tc.latestMu.Lock()
	defer tc.latestMu.Unlock()
	tc.latestThermalControlEvent = tce
	return tce, nil
}
