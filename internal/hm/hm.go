package hm

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"sync"

	"github.com/stapelberg/hmgo/internal/bidcos"
)

const (
	Mask1Bit = 0x1
	Mask2Bit = 0x3
	Mask3Bit = 0x7
	Mask4Bit = 0xF
	Mask5Bit = 0x1F
	Mask6Bit = 0x3F
	Mask7Bit = 0x7F
	Mask8Bit = 0xFF
)

type Device interface {
	Channels() int
	Pair() error
	MostRecentEvents() []Event
	AddrHex() string
	Name() string
}

type Event interface {
	HTML() template.HTML
}

// FullyQualifiedChannel identifies a channel at a specific
// BidCoS-addressed peer.
type FullyQualifiedChannel struct {
	Peer    [3]byte
	Channel byte
}

// StandardDevice encapsulates behavior shared by all HomeMatic
// devices.
type StandardDevice struct {
	BCS       *bidcos.Sender
	Addr      [3]byte
	addrHex   string
	HumanName string

	once sync.Once

	// msgcnt is incremented and accessed via count()
	msgcnt byte

	NumChannels int
}

func (sd *StandardDevice) Name() string {
	return sd.HumanName
}

func (sd *StandardDevice) Channels() int {
	return sd.NumChannels
}

func (sd *StandardDevice) count() byte {
	result := sd.msgcnt
	sd.msgcnt += 9
	return result
}

func (sd *StandardDevice) AddrHex() string {
	sd.once.Do(func() {
		sd.addrHex = fmt.Sprintf("%x", sd.Addr)
	})
	return sd.addrHex
}

func (sd *StandardDevice) String() string {
	return fmt.Sprintf("[BidCoS:%s]", sd.AddrHex())
}

func (sd *StandardDevice) ConfigStart(channel, paramlist byte) error {
	return sd.BCS.WritePacket(&bidcos.Packet{
		Msgcnt: sd.count(),
		Flags:  bidcos.DefaultFlags,
		Cmd:    bidcos.Config,
		Dest:   sd.Addr,
		Payload: []byte{
			channel,
			bidcos.ConfigStart,
			0, 0, 0, // peer address
			0, // peer channel
			paramlist,
		},
	})
}

func (sd *StandardDevice) ConfigWriteIndex(channel byte, kv []byte) error {
	return sd.BCS.WritePacket(&bidcos.Packet{
		Msgcnt: sd.count(),
		Flags:  bidcos.DefaultFlags,
		Cmd:    bidcos.Config,
		Dest:   sd.Addr,
		Payload: append([]byte{
			channel,
			bidcos.ConfigWriteIndexPairs,
		}, kv...),
	})
}

func (sd *StandardDevice) ConfigEnd(channel byte) error {
	return sd.BCS.WritePacket(&bidcos.Packet{
		Msgcnt: sd.count(),
		Flags:  bidcos.DefaultFlags,
		Cmd:    bidcos.Config,
		Dest:   sd.Addr,
		Payload: []byte{
			channel,
			bidcos.ConfigEnd,
		},
	})
}

func (sd *StandardDevice) Pair() error {
	if err := sd.ConfigStart(0, 0); err != nil {
		return err
	}
	if err := sd.ConfigWriteIndex(0, []byte{
		0x02, 0x01, // internal keys not visible
		0x0a, sd.BCS.Addr[0],
		0x0b, sd.BCS.Addr[1],
		0x0c, sd.BCS.Addr[2],
	}); err != nil {
		return err
	}
	return sd.ConfigEnd(0)
}

func (sd *StandardDevice) ConfigParamReq(channel, paramlist byte) error {
	return sd.BCS.WritePacket(&bidcos.Packet{
		Msgcnt: sd.count(),
		Flags:  bidcos.DefaultFlags | bidcos.Burst,
		Cmd:    bidcos.Config,
		Dest:   sd.Addr,
		Payload: []byte{
			channel, // channel
			bidcos.ConfigParamReq,
			0, 0, 0, // peer address
			0,         // peer channel
			paramlist, // param list
		},
	})
}

func (sd *StandardDevice) ConfigPeerListReq(channel byte) error {
	return sd.BCS.WritePacket(&bidcos.Packet{
		Msgcnt: sd.count(),
		Flags:  bidcos.DefaultFlags | bidcos.Burst,
		Cmd:    bidcos.Config,
		Dest:   sd.Addr,
		Payload: []byte{
			channel, // channel
			bidcos.ConfigPeerListReq,
		},
	})
}

func (sd *StandardDevice) ConfigPeerAdd(channel byte, peerAddr [3]byte, peerChannel byte) error {
	return sd.BCS.WritePacket(&bidcos.Packet{
		Msgcnt: sd.count(),
		Flags:  bidcos.DefaultFlags | bidcos.Burst,
		Cmd:    bidcos.Config,
		Dest:   sd.Addr,
		Payload: []byte{
			channel, // channel
			bidcos.ConfigPeerAdd,
			peerAddr[0], peerAddr[1], peerAddr[2],
			peerChannel, // peer channel a
			0x00,        // peer channel b
		},
	})
}

func (sd *StandardDevice) ConfigPeerRemove(channel byte, peerAddr [3]byte, peerChannel byte) error {
	return sd.BCS.WritePacket(&bidcos.Packet{
		Msgcnt: sd.count(),
		Flags:  bidcos.DefaultFlags | bidcos.Burst,
		Cmd:    bidcos.Config,
		Dest:   sd.Addr,
		Payload: []byte{
			channel, // channel
			bidcos.ConfigPeerRemove,
			peerAddr[0], peerAddr[1], peerAddr[2],
			peerChannel, // peer channel a
			0x00,        // peer channel b
		},
	})
}

// LoadConfig is a convenience function to load the device parameters
// in paramlist of channel into mem.
func (sd *StandardDevice) LoadConfig(mem []byte, channel, paramlist byte) error {
	if err := sd.ConfigParamReq(channel, paramlist); err != nil {
		return err
	}
ReadConfig:
	for {
		pkt, err := sd.BCS.ReadPacket()
		if err != nil {
			return err
		}
		if !bytes.Equal(pkt.Source[:], sd.Addr[:]) {
			log.Printf("dropping BidCoS packet from different device: %+v", pkt)
			continue
		}
		p := pkt.Payload // for convenience
		switch p[0] {
		case bidcos.InfoParamResponsePairs:
			if bytes.Equal(p[1:], []byte{0x00, 0x00}) {
				break ReadConfig
			}
			// idx/val byte pairs
			for i := 1; i < len(p); i += 2 {
				mem[p[i]] = p[i+1]
			}

		case bidcos.InfoParamResponseSeq:
			if p[1] == 0x00 {
				break ReadConfig
			}
			for i := 0; i < len(p)-2; i++ {
				mem[p[1]+byte(i)] = p[2+i]
			}

		default:
			return fmt.Errorf("unexpected ConfigParamReq reply: %x", p)
		}
	}
	return nil
}

// EnsureConfigured is a convenience function.
func (sd *StandardDevice) EnsureConfigured(channel, paramlist byte, cb func([]byte) error) error {
	// config memory is indexed using a byte, i.e. capped at 256
	devmem := make([]byte, 256)
	if err := sd.LoadConfig(devmem, channel, paramlist); err != nil {
		return err
	}

	target := make([]byte, len(devmem))
	copy(target, devmem)

	if err := cb(target); err != nil {
		return err
	}

	var pairs []byte
	for i := 0; i < len(devmem); i++ {
		if devmem[i] != target[i] {
			pairs = append(pairs, byte(i), target[i])
		}
	}

	if len(pairs) == 0 {
		return nil
	}

	// BidCoS frames have a maximum length of 16 bytes. A
	// ConfigWriteIndex packet has 2 bytes overhead, so we send
	// key/value pairs in blocks of 14 bytes each.
	pkts := len(pairs) / 14
	if len(pairs)%14 > 0 {
		pkts++
	}

	log.Printf("need to update: %x", pairs)

	if err := sd.ConfigStart(channel, paramlist); err != nil {
		return err
	}
	for i := 0; i < pkts; i++ {
		offset := 14 * i
		rest := len(pairs) - offset
		if rest > 14 {
			rest = 14
		}
		log.Printf("sending packet %d: %x", i, pairs[offset:offset+rest])
		if err := sd.ConfigWriteIndex(0, pairs[offset:offset+rest]); err != nil {
			return err
		}
	}
	if err := sd.ConfigEnd(channel); err != nil {
		return err
	}

	return nil
}

var endOfPeerList = []byte{0x00, 0x00, 0x00, 0x00}

func (sd *StandardDevice) EnsurePeeredWith(channel byte, dest FullyQualifiedChannel) error {
	if err := sd.ConfigPeerListReq(channel); err != nil {
		return err
	}

	var peers []FullyQualifiedChannel

ReadPeers:
	for {
		pkt, err := sd.BCS.ReadPacket()
		if err != nil {
			return err
		}

		if !bytes.Equal(pkt.Source[:], sd.Addr[:]) {
			log.Printf("dropping BidCoS packet from different device: %+v", pkt)
			continue
		}

		if pkt.Payload[0] != 0x01 /* INFO_PEER_LIST */ {
			return fmt.Errorf("unexpected payload: %x", pkt.Payload[0])
		}

		list := pkt.Payload[1:]
		for i := 0; i < len(list)/4; i++ {
			off := 4 * i
			if bytes.Equal(list[off:off+4], endOfPeerList) {
				break ReadPeers
			}
			var p FullyQualifiedChannel
			copy(p.Peer[:], list[off:off+3])
			p.Channel = list[off+3]
			peers = append(peers, p)
		}
	}

	log.Printf("%v has existing peers %+v", sd, peers)
	if len(peers) > 1 {
		// TODO(later): unpeer everything
		return fmt.Errorf("unpeering not yet implemented")
	}
	if len(peers) == 1 {
		existing := peers[0]
		if bytes.Equal(existing.Peer[:], dest.Peer[:]) {
			return nil
		}

		log.Printf("removing existing peer %v", existing)
		if err := sd.ConfigPeerRemove(channel, existing.Peer, existing.Channel); err != nil {
			return err
		}

		pkt, err := sd.BCS.ReadPacket()
		if err != nil {
			return err
		}
		if got, want := pkt.Cmd, byte(0x10); got != want {
			return fmt.Errorf("unexpected response command: got %x, want %x", got, want)
		}
		if got, want := len(pkt.Payload), 1; got < want {
			return fmt.Errorf("unexpected response payload length: got %d, want >= %d", got, want)
		}
		if got, want := pkt.Payload[0], byte(0x00); got != want {
			return fmt.Errorf("unexpected acknowledgement status: got %x, want %x", got, want)
		}

		// fallthrough to add the peer
	}

	log.Printf("adding peer %v", dest)
	if err := sd.ConfigPeerAdd(channel, dest.Peer, dest.Channel); err != nil {
		return err
	}

	pkt, err := sd.BCS.ReadPacket()
	if err != nil {
		return err
	}
	if got, want := pkt.Cmd, byte(0x02); got != want {
		return fmt.Errorf("unexpected response command: got %x, want %x", got, want)
	}
	if got, want := len(pkt.Payload), 1; got < want {
		return fmt.Errorf("unexpected response payload length: got %d, want >= %d", got, want)
	}
	if got, want := pkt.Payload[0], byte(0x00); got != want {
		return fmt.Errorf("unexpected acknowledgement status: got %x, want %x", got, want)
	}

	return nil
}
