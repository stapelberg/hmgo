package power

import (
	"sync"

	"github.com/stapelberg/hmgo/internal/bidcos"
	"github.com/stapelberg/hmgo/internal/hm"
)

const (
	PowerMeter    = 0x02
	ChannelSwitch = 0x01
	ChannelMaster = 0x05

	On  = 0xc8
	Off = 0x00
)

const (
	MaintenanceChannel = iota
	SwitchChannel
	ConditionPowermeterChannel
	ConditionPowerChannel
	ConditionCurrentChannel
	ConditionVoltageChannel
	ConditionFrequencyChannel
)

type PowerSwitch struct {
	hm.StandardDevice

	latestPowerEvent *PowerEvent
	latestMu         sync.RWMutex
}

func (ps *PowerSwitch) HomeMaticType() string { return "power" }

func NewPowerSwitch(sd hm.StandardDevice) *PowerSwitch {
	sd.NumChannels = 6
	return &PowerSwitch{StandardDevice: sd}
}

func (ps *PowerSwitch) MostRecentEvents() []hm.Event {
	var result []hm.Event
	ps.latestMu.RLock()
	defer ps.latestMu.RUnlock()

	if ps.latestPowerEvent != nil {
		result = append(result, ps.latestPowerEvent)
	}

	return result
}

func (ps *PowerSwitch) LevelSet(channel, state, onTime byte) error {
	return ps.BCS.WritePacket(&bidcos.Packet{
		Flags: bidcos.DefaultFlags,
		Cmd:   0x11, // LevelSet
		Dest:  ps.Addr,
		Payload: []byte{
			0x02, // subtype
			channel,
			state,
			0x00, // constant
			onTime,
		},
	})
}
