package heating

import (
	"sync"

	"github.com/stapelberg/hmgo/internal/hm"
)

// channels
const (
	ClimateControlReceiver = 0x02
)

// Thermostat represents a HM-CC-RT-DN heating thermostat. Its manual
// can be found at
// http://www.eq-3.de/Downloads/eq3/downloads_produktkatalog/homematic/bda/HM-CC-RT-DN_UM_GE_eQ-3_web.pdf
type Thermostat struct {
	hm.StandardDevice

	latestInfoEvent *InfoEvent
	latestMu        sync.RWMutex
}

func (t *Thermostat) HomeMaticType() string { return "heating" }

func NewThermostat(sd hm.StandardDevice) *Thermostat {
	sd.NumChannels = 6
	return &Thermostat{StandardDevice: sd}
}

func (t *Thermostat) MostRecentEvents() []hm.Event {
	var result []hm.Event
	t.latestMu.RLock()
	defer t.latestMu.RUnlock()

	if t.latestInfoEvent != nil {
		result = append(result, t.latestInfoEvent)
	}

	return result
}
