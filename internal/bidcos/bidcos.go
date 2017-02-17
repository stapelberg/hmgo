// Package bidcos implements the HomeMatic BidCoS (bidirectional
// communication standard) radio protocol.
package bidcos

import (
	"fmt"
	"io"
	"sync"
)

// cmd is top-level (e.g. SET), frames usually specify a subtype (e.g. MANU_MODE_SET)

// BidCoS commands
const (
	DeviceInfo byte = iota
	Config
	Ack
	Info             = 0x10
	ClimateEvent     = 0x58
	ThermalControl   = 0x5a
	PowerEventCyclic = 0x5e
	PowerEvent       = 0x5f
	WeatherEvent     = 0x70
)

// BidCoS Config subcommands
const (
	_ byte = iota
	ConfigPeerAdd
	ConfigPeerRemove
	ConfigPeerListReq
	ConfigParamReq
	ConfigStart
	ConfigEnd
	ConfigWriteIndexSeq
	ConfigWriteIndexPairs
	ConfigSerialReq
	ConfigPairSerial
	_
	_
	_
	ConfigStatusRequest
)

// BidCoS Info subcommands
const (
	InfoSerial byte = iota
	InfoPeerList
	InfoParamResponsePairs
	InfoParamResponseSeq
	InfoParamChange
	_
	InfoActuatorStatus
	InfoTemp = 0x0a
)

// Packet flags
const (
	// Wake up the destination device from power-save mode.
	WakeUp byte = 1 << iota
	// Device is awake, send messages now.
	WakeMeUp
	// Send message to all devices.
	Broadcast
	_
	// Wake up the destination device from power-save mode.
	Burst
	// Bi-directional, i.e. response expected.
	BiDi
	// Packet was repeated (not seen in the wild).
	Repeated
	// Packet can be repeated (always set).
	RepeatEnable
)

const DefaultFlags = RepeatEnable | BiDi

// Packet is a BidCoS packet.
type Packet struct {
	status  uint8
	info    uint8
	rssi    uint8
	Msgcnt  uint8
	Flags   uint8 // see Packet flags above
	Cmd     uint8 // see BidCoS commands above
	Source  [3]byte
	Dest    [3]byte
	Payload []byte // at most 17 bytes
}

var messageCounter struct {
	counter byte
	sync.RWMutex
}

func (p *Packet) Encode() []byte {
	// c.f. https://svn.fhem.de/trac/browser/trunk/fhem/FHEM/00_HMUARTLGW.pm?rev=13367#L1464
	// c.f. https://github.com/Homegear/Homegear-HomeMaticBidCoS/blob/5255288954f3da42e12fa72a06963b99089d323f/src/PhysicalInterfaces/Hm-Mod-Rpi-Pcb.cpp#L858
	messageCounter.Lock()
	defer messageCounter.Unlock()
	cnt := p.Msgcnt
	if cnt == 0 {
		cnt = messageCounter.counter
		// The Homematic CCU2 increments its message counter by 9 between
		// each message. My guess is that the resulting pattern has better
		// radio characteristics.
		messageCounter.counter += 9
	}
	var burst byte
	if p.Flags&0x10 == 0x10 {
		burst = 0x01
	}
	res := []byte{
		0x00, // status
		0x00, // info
		burst,
		cnt,
		p.Flags,
		p.Cmd,
	}
	res = append(res, p.Source[:]...)
	res = append(res, p.Dest[:]...)
	res = append(res, p.Payload...)
	return res
}

func Decode(b []byte) (*Packet, error) {
	if got, want := len(b), 12; got < want {
		return nil, fmt.Errorf("too short for a bidcos packet: got %d, want >= %d", got, want)
	}

	// TODO(later): decode RSSI, see Homegear-HomeMaticBidCoS/src/BidCoSPacket.cpp

	return &Packet{
		status:  b[0],
		info:    b[1],
		rssi:    b[2],
		Msgcnt:  b[3],                        // hg: “message counter”
		Flags:   b[4],                        // hg: “control byte”
		Cmd:     b[5],                        // hg: “message type”
		Source:  [3]byte{b[6], b[7], b[8]},   // hg: “senderAddress”
		Dest:    [3]byte{b[9], b[10], b[11]}, // hg: “destinationAddress”
		Payload: b[12:],
	}, nil
}

type Gateway interface {
	io.ReadWriter
	Confirm() error
}

// Sender is a convenience wrapper around a Gateway which fills in the
// BidCoS source address for outgoing packets, automatically confirms
// outgoing packets and decodes incoming packets.
type Sender struct {
	Gateway Gateway
	Addr    [3]byte
}

func NewSender(gw Gateway, addr [3]byte) (*Sender, error) {
	if got, want := len(addr), 3; got != want {
		return nil, fmt.Errorf("unexpected address length: got %d, want %d", got, want)
	}
	return &Sender{
		Gateway: gw,
		Addr:    addr,
	}, nil
}

func (s *Sender) ReadPacket() (*Packet, error) {
	// 17 byte BidCoS maximum observed payload + 12 bytes fixed BidCoS overhead
	var buf [17 + 12]byte
	n, err := s.Gateway.Read(buf[:])
	if err != nil {
		return nil, err
	}
	return Decode(buf[:n])
}

func (s *Sender) WritePacket(pkt *Packet) error {
	pkt.Source = s.Addr
	//log.Printf("writing bidcos packet %+v", pkt)
	_, err := s.Gateway.Write(pkt.Encode())
	if err != nil {
		return err
	}
	return s.Gateway.Confirm()
}
