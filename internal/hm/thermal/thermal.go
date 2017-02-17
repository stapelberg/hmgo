package thermal

import (
	"sync"
	"time"

	"github.com/stapelberg/homematic/internal/hm"
)

const prometheusNamespace = "hmthermal"

// channels
const (
	ThermalControlTransmit = 0x02
)

const (
	WeekdayMask = 1<<uint(time.Monday) |
		1<<uint(time.Tuesday) |
		1<<uint(time.Wednesday) |
		1<<uint(time.Thursday) |
		1<<uint(time.Friday)
	WeekendMask = 1<<uint(time.Saturday) | 1<<uint(time.Sunday)
)

type Program struct {
	DayMask  int
	Endtimes [13]ProgramEntry
}

type ProgramEntry struct {
	Endtime     uint64
	Temperature float64
}

// ThermalControl represents a HM-TC-IT-WM-W-EU remote temperature
// sensor and thermostat control unit.
//
// Its manual can be found at
// http://www.eq-3.de/Downloads/eq3/downloads_produktkatalog/homematic/bda/HM-TC-IT-WM-W-EU_UM_GE_eQ-3_web.pdf
//
// Further details can be found at
// https://wiki.fhem.de/wiki/HM-TC-IT-WM-W-EU_Funk-Wandthermostat_AP
type ThermalControl struct {
	hm.StandardDevice

	latestWeatherEvent        *WeatherEvent
	latestThermalControlEvent *ThermalControlEvent
	latestInfoEvent           *InfoEvent
	latestMu                  sync.RWMutex
}

func NewThermalControl(sd hm.StandardDevice) *ThermalControl {
	sd.NumChannels = 7
	return &ThermalControl{StandardDevice: sd}
}

func (tc *ThermalControl) MostRecentEvents() []hm.Event {
	var result []hm.Event
	tc.latestMu.RLock()
	defer tc.latestMu.RUnlock()

	if tc.latestWeatherEvent != nil {
		result = append(result, tc.latestWeatherEvent)
	}

	if tc.latestThermalControlEvent != nil {
		result = append(result, tc.latestThermalControlEvent)
	}

	if tc.latestInfoEvent != nil {
		result = append(result, tc.latestInfoEvent)
	}

	return result
}

func (tc *ThermalControl) encodeProgramDay(entries [13]ProgramEntry) []byte {
	result := make([]byte, 26)

	for i := 0; i < 13; i++ {
		endtime := entries[i].Endtime
		if endtime == 0 {
			endtime = 1440
		}
		temperature := entries[i].Temperature
		if temperature == 0.0 {
			temperature = 17.0
		}
		result[(2 * i)] = byte(((uint16(endtime/5) & 0x0100) >> 8) | ((uint16(temperature*2.0) & hm.Mask6Bit) << 1))
		result[(2*i)+1] = byte(((uint16(endtime/5) & 0x00FF) >> 0))
	}

	return result
}

// programOffsets maps weekdays to device memory location offsets
var programOffsets = map[time.Weekday]int{
	time.Saturday:  20,
	time.Sunday:    46,
	time.Monday:    72,
	time.Tuesday:   98,
	time.Wednesday: 124,
	time.Thursday:  150,
	time.Friday:    176,
}

func (tc *ThermalControl) SetPrograms(mem []byte, programs []Program) {
	for _, day := range []time.Weekday{
		time.Saturday,
		time.Sunday,
		time.Monday,
		time.Tuesday,
		time.Wednesday,
		time.Thursday,
		time.Friday,
	} {
		for _, pg := range programs {
			if 1<<uint(day)&pg.DayMask != 0 {
				copy(mem[programOffsets[day]:], tc.encodeProgramDay(pg.Endtimes))
			}
		}
	}
}
