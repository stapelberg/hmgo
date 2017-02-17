package thermal

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	weatherEventTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "WeatherEventTemperature",
			Help:      "Temperature in degC",
		},
		[]string{"address", "name"})

	weatherEventHumidity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "WeatherEventHumidity",
			Help:      "Humidity in percentage points",
		},
		[]string{"address", "name"})
)

func init() {
	prometheus.MustRegister(weatherEventTemperature)
	prometheus.MustRegister(weatherEventHumidity)
}

type WeatherEvent struct {
	Temperature float64 // in degC
	Humidity    uint64  // in percentage points
}

var weTmpl = template.Must(template.New("weatherevent").Parse(`
<strong>Weather:</strong><br>
Temperature: {{ .Temperature }} ℃<br>
Humidity: {{ .Humidity }}%<br>
`))

func (we *WeatherEvent) HTML() template.HTML {
	var buf bytes.Buffer
	if err := weTmpl.Execute(&buf, we); err != nil {
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return template.HTML(buf.String())
}

func (tc *ThermalControl) DecodeWeatherEvent(payload []byte) (*WeatherEvent, error) {
	// c.f. <frame id="WEATHER_EVENT"> in rftypes/tc.xml
	if got, want := len(payload), 3; got != want {
		return nil, fmt.Errorf("unexpected payload size: got %d, want %d", got, want)
	}
	we := &WeatherEvent{
		Temperature: float64((int16(payload[0])<<8|int16(payload[1]))&0x3FFF) / 10,
		Humidity:    uint64(payload[2]),
	}

	weatherEventTemperature.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(we.Temperature)
	weatherEventHumidity.With(prometheus.Labels{"name": tc.Name(), "address": tc.AddrHex()}).Set(float64(we.Humidity))

	tc.latestMu.Lock()
	defer tc.latestMu.Unlock()
	tc.latestWeatherEvent = we
	return we, nil
}
