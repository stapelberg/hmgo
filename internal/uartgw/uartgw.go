// Package uartgw implements communicating with a HM-MOD-RPI-PCB
// HomeMatic gateway.
/*

The HM-MOD-RPI-PCB uses a frame-based protocol. When reading frames,
0xfc is an escape byte and needs to be replaced:

    0xfc 0x7d represents 0xfd
    0xfc 0x7c represents 0xfc

This technique results in 0xfd always meaning “start of a frame”,
which means we can re-synchronize on 0xfd after reading invalid data.

Each frame has the following format:

    uint8  frame delimiter (always 0xfd)
    uint16 length (big endian)
    []byte packet
    uint16 crc (big endian)

See the bidcosTable variable for the specific CRC16 parameters.

Each packet has the following format:

    uint8  destination (see uartdest)
    uint8  message counter
    uint8  command (see uartcmd)
    []byte payload

Note that command values depend on the state of the UARTGW, i.e. the
same value means something different in bootloader state
vs. application code state.

*/
package uartgw

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/sigurn/crc16"
)

type deviceState uint8

type UARTGW struct {
	FirmwareVersion string
	SerialNumber    string

	// HMID is the HomeMatic ID of the UARTGW and must not be changed.
	HMID [3]byte

	uart     io.ReadWriter
	msgcnt   uint8
	devstate uartdest
}

// NewUARTGW initializes a UARTGW which is expected to have just been reset.
func NewUARTGW(uart io.ReadWriter, HMID [3]byte, now time.Time) (*UARTGW, error) {
	gw := &UARTGW{
		uart: uart,
		HMID: HMID,
	}
	return gw, gw.init(now)
}

type uartdest uint8

const (
	OS      uartdest = 0
	App              = 1
	Dual             = 254
	DualErr          = 255
)

func (u uartdest) String() string {
	switch u {
	case OS:
		return "OS"
	case App:
		return "App"
	case Dual:
		return "Dual"
	case DualErr:
		return "DualErr"
	default:
		return "<invalid dest>"
	}
}

type uartcmd uint8

// c.f. https://svn.fhem.de/trac/browser/trunk/fhem/FHEM/00_HMUARTLGW.pm?rev=13367#L23
// NOTE(stapelberg): I’m not convinced the mental model of these
// constants is entirely correct. For example, we send OSGetSerial
// while the device is in the App state, which doesn’t make sense to
// me.
const (
	// While UARTGW is in state Bootloader
	OSGetApp uartcmd = iota
	OSGetFirmware
	OSChangeApp
	OSAck
	OSUpdateFirmware
	OSNormalMode
	OSUpdateMode
	OSGetCredits
	OSEnableCredits
	OSEnableCSMACA
	OSGetSerial
	OSSetTime

	AppSetHMID
	AppGetHMID
	AppSend
	AppSetCurrentKey
	AppAck
	AppRecv
	AppAddPeer
	AppRemovePeer
	AppGetPeers
	AppPeerAddAES
	AppPeerRemoveAES
	AppSetTempKey
	AppSetPreviousKey
	AppDefaultHMID

	DualGetApp
	DualChangeApp
)

func (u *UARTGW) Command(cmd uint8) (uartcmd, error) {
	switch u.devstate {
	case OS:
		switch cmd {
		case 0x00:
			return OSGetApp, nil
		case 0x02:
			return OSGetFirmware, nil
		case 0x03:
			return OSChangeApp, nil
		case 0x04:
			return OSAck, nil
		case 0x05:
			return OSUpdateFirmware, nil
		case 0x06:
			return OSNormalMode, nil
		case 0x07:
			return OSUpdateMode, nil
		case 0x08:
			return OSGetCredits, nil
		case 0x09:
			return OSEnableCredits, nil
		case 0x0a:
			return OSEnableCSMACA, nil
		case 0x0b:
			return OSGetSerial, nil
		case 0x0e:
			return OSSetTime, nil

		default:
			return OSGetApp, fmt.Errorf("unknown command: %v (state %v)", cmd, u.devstate)
		}

	case App:
		switch cmd {
		case 0x00:
			// XXX: not sure if AppSetHMID can ever be received from
			// the device, but suddenly receiving 0x00 might mean the
			// coprocessor entered the bootloader again.
			return OSGetApp, nil
			//return AppSetHMID
		case 0x01:
			return AppGetHMID, nil
		case 0x02:
			return AppSend, nil
		case 0x03:
			return AppSetCurrentKey, nil
		case 0x04:
			return AppAck, nil
		case 0x05:
			return AppRecv, nil
		case 0x06:
			return AppAddPeer, nil
		case 0x07:
			return AppRemovePeer, nil
		case 0x08:
			return AppGetPeers, nil
		case 0x09:
			return AppPeerAddAES, nil
		case 0x0a:
			return AppPeerRemoveAES, nil
		case 0x0b:
			return AppSetTempKey, nil
		case 0x0f:
			return AppSetPreviousKey, nil
		case 0x10:
			return AppDefaultHMID, nil

		default:
			return OSGetApp, fmt.Errorf("unknown command: %v (state %v)", cmd, u.devstate)
		}
	}
	return OSGetApp, fmt.Errorf("unknown device state: %v", u.devstate)
}

func (c uartcmd) Byte() (byte, error) {
	switch c {
	case OSGetApp:
		return 0x00, nil
	case OSGetFirmware:
		return 0x02, nil
	case OSChangeApp:
		return 0x03, nil
	case OSAck:
		return 0x04, nil
	case OSUpdateFirmware:
		return 0x05, nil
	case OSNormalMode:
		return 0x06, nil
	case OSUpdateMode:
		return 0x07, nil
	case OSGetCredits:
		return 0x08, nil
	case OSEnableCredits:
		return 0x09, nil
	case OSEnableCSMACA:
		return 0x0a, nil
	case OSGetSerial:
		return 0x0b, nil
	case OSSetTime:
		return 0x0e, nil

	case AppSetHMID:
		return 0x00, nil
	case AppSend:
		return 0x02, nil
	case AppSetCurrentKey:
		return 0x03, nil
	case AppAck:
		return 0x04, nil
	case AppRecv:
		return 0x05, nil
	case AppAddPeer:
		return 0x06, nil
	case AppGetPeers:
		return 0x08, nil
	case AppPeerRemoveAES:
		return 0x0a, nil
	}
	return 0x00, fmt.Errorf("unknown command: %v", c)
}

func (c uartcmd) String() string {
	switch c {
	case OSGetApp:
		return "OSGetApp"
	case OSGetFirmware:
		return "OSGetFirmware"
	case OSChangeApp:
		return "OSChangeApp"
	case OSAck:
		return "OSAck"
	case OSUpdateFirmware:
		return "OSUpdateFirmware"
	case OSNormalMode:
		return "OSNormalMode"
	case OSUpdateMode:
		return "OSUpdateMode"
	case OSGetCredits:
		return "OSGetCredits"

	case AppAck:
		return "AppAck"
	case AppRecv:
		return "AppRecv"

	default:
		return fmt.Sprintf("<invalid cmd (%x)>", uint8(c))
	}

}

// Packet is a package received from the HM-MOD-RPI-PCB serial gateway (“UARTGW”).
type Packet struct {
	Dst     uartdest
	msgcnt  uint8
	Cmd     uartcmd
	Payload []byte
}

func (u Packet) String() string {
	return fmt.Sprintf("dest: %s\nmsgcnt: %d\ncmd: %s", u.Dst, u.msgcnt, u.Cmd)
}

var bidcosTable = crc16.MakeTable(crc16.Params{
	Poly:   0x8005,
	Init:   0xd77f,
	RefIn:  false,
	RefOut: false,
	XorOut: 0x0000,
	Check:  0x0000,
	Name:   "BidCoS",
})

func (u *UARTGW) ReadPacket() (*Packet, error) {
	var fullpkt bytes.Buffer
	r := io.TeeReader(&unescapingReader{r: u.uart}, &fullpkt)

	for {
		fullpkt.Reset()

		b := make([]byte, 1)
		if _, err := r.Read(b); err != nil {
			return nil, err
		}
		if b[0] != 0xfd {
			log.Printf("skipping non-frame-delimiter byte %x", b[0])
			continue
		}

		// Get packet length, read payload
		var length uint16
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		var payload bytes.Buffer
		if _, err := io.CopyN(&payload, r, int64(length)); err != nil {
			return nil, err
		}

		// Calculate and verify checksum
		want := crc16.Checksum(fullpkt.Bytes(), bidcosTable)
		var got uint16
		if err := binary.Read(r, binary.BigEndian, &got); err != nil {
			return nil, err
		}
		if got != want {
			return nil, fmt.Errorf("unexpected checksum: got %x, want %x", got, want)
		}

		// Parse packet
		frame := payload.Bytes()
		cmd, err := u.Command(frame[2])
		if err != nil {
			return nil, err
		}
		// log.Printf("frame with length = %d, full = %x, content = %x, string = %s", length, fullpkt.Bytes(), frame, string(frame))
		return &Packet{
			Dst:     uartdest(frame[0]),
			msgcnt:  uint8(frame[1]),
			Cmd:     cmd,
			Payload: frame[3:],
		}, nil
	}
}

func (u *UARTGW) WritePacket(pkt *Packet) error {
	var fullpkt bytes.Buffer
	w := io.MultiWriter(u.uart, &fullpkt)
	if _, err := w.Write([]byte{0xfd}); err != nil {
		return err
	}
	// Now that the frame is introduced, start escaping
	esc := escapingWriter{w: u.uart}
	w = io.MultiWriter(&esc, &fullpkt)
	length := uint16(3 + len(pkt.Payload))
	if err := binary.Write(w, binary.BigEndian, &length); err != nil {
		return err
	}
	cmd, err := pkt.Cmd.Byte()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte{byte(pkt.Dst), byte(u.msgcnt), cmd}); err != nil {
		return err
	}
	u.msgcnt++
	if _, err := w.Write(pkt.Payload); err != nil {
		return err
	}
	// log.Printf("wrote %x", fullpkt.Bytes())
	return binary.Write(&esc, binary.BigEndian, crc16.Checksum(fullpkt.Bytes(), bidcosTable))
}

func (u *UARTGW) init(now time.Time) error {
	// on the wire: FD000C000000436F5F4350555F424C7251
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}
	if got, want := pkt.Cmd, OSGetApp; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}
	if got, want := string(pkt.Payload), "Co_CPU_BL"; got != want {
		return fmt.Errorf("unexpected UARTGW application: got %q, want %q", got, want)
	}

	if err := u.switchToApp(); err != nil {
		return fmt.Errorf("switching from bootloader to application: %v", err)
	}

	if err := u.getFirmwareVersion(); err != nil {
		return fmt.Errorf("getting firmware version: %v", err)
	}

	if err := u.enableCSMACA(); err != nil {
		return fmt.Errorf("enabling CSMA/CA: %v", err)
	}

	if err := u.getSerialNumber(); err != nil {
		return fmt.Errorf("getting serial number: %v", err)
	}

	if err := u.SetTime(now); err != nil {
		return fmt.Errorf("setting time: %v", err)
	}

	if err := u.setCurrentKey(); err != nil {
		return fmt.Errorf("setting current key: %v", err)
	}

	if err := u.setHMID(); err != nil {
		return fmt.Errorf("setting HMID: %v", err)
	}

	return nil
}

// switchToApp switches from bootloader to application code.
func (u *UARTGW) switchToApp() error {
	// on the wire: FD0003000003180A
	if err := u.WritePacket(&Packet{Cmd: OSChangeApp}); err != nil {
		return err
	}

	// on the wire: FD000400000401993D
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}
	if got, want := pkt.Cmd, OSAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	// on the wire: FD000D000000436F5F4350555F417070D831
	pkt, err = u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, OSGetApp; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	if got, want := string(pkt.Payload), "Co_CPU_App"; got != want {
		return fmt.Errorf("unexpected UARTGW application: got %q, want %q", got, want)
	}

	u.devstate = App

	return nil
}

func (u *UARTGW) getFirmwareVersion() error {
	// on the wire: FD00030001021E0C
	if err := u.WritePacket(&Packet{Cmd: OSGetFirmware}); err != nil {
		return err
	}

	// on the wire: FD000A00010402010003010201AA8A
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	version := pkt.Payload[4:]
	u.FirmwareVersion = fmt.Sprintf("%d.%d.%d",
		uint8(version[0]),
		uint8(version[1]),
		uint8(version[2]))

	return nil
}

// enableCSMACA enables Carrier sense multiple access with collision avoidance
func (u *UARTGW) enableCSMACA() error {
	// on the wire: FD000400020A003D10
	if err := u.WritePacket(&Packet{
		Cmd:     OSEnableCSMACA,
		Payload: []byte{0x01}}); err != nil {
		return err
	}

	// on the wire: FD0004000204011916
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	return nil
}

func (u *UARTGW) getSerialNumber() error {
	// on the wire: FD000300030B9239
	if err := u.WritePacket(&Packet{Cmd: OSGetSerial}); err != nil {
		return err
	}

	// on the wire: FD000E000304024E4551313333303938306AB9
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	u.SerialNumber = string(pkt.Payload[1:])

	return nil
}

func (u *UARTGW) SetTime(now time.Time) error {
	// on the wire: FD000800040E58A7116300548E
	secsSinceEpoch := uint32(now.Unix())
	var timePayload bytes.Buffer
	if err := binary.Write(&timePayload, binary.BigEndian, secsSinceEpoch); err != nil {
		return err
	}
	_, offset := now.Zone()
	if _, err := timePayload.Write([]byte{byte(offset / 1800)}); err != nil {
		return err
	}
	if err := u.WritePacket(&Packet{Cmd: OSSetTime, Payload: timePayload.Bytes()}); err != nil {
		return err
	}

	// on the wire: FD000400040401196E
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	return nil
}

func (u *UARTGW) setCurrentKey() error {
	// on the wire: FD001401050300112233445566778899AABBCCDDEEFF024C6D
	keyPayload := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
		02, // key index
	}
	if err := u.WritePacket(&Packet{
		Dst:     App,
		Cmd:     AppSetCurrentKey,
		Payload: keyPayload,
	}); err != nil {
		return err
	}

	// on the wire: FD0004010504010D7A
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	return nil
}

func (u *UARTGW) setHMID() error {
	// on the wire: FD0006010600FC7DB02CD166
	if err := u.WritePacket(&Packet{
		Dst:     App,
		Cmd:     AppSetHMID,
		Payload: u.HMID[:],
	}); err != nil {
		return err
	}

	// on the wire: FD0004010604010D46
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	return nil
}

func (u *UARTGW) AddPeer(addr []byte, channels int) error {
	// Repeat the message twice because the CCU2 does that
	// (cargo-culted from homegear).
	for i := 0; i < 2; i++ {
		// Add peer / get peer config
		// on the wire: FD000901080640C2A8000000022E
		addPeerPayload := [6]byte{
			addr[0], addr[1], addr[2],
			0x00, 0x00, 0x00,
		}
		if err := u.WritePacket(&Packet{
			Dst:     App,
			Cmd:     AppAddPeer,
			Payload: addPeerPayload[:],
		}); err != nil {
			return err
		}

		// on the wire: FD00100108040701010001FFFFFFFFFFFFFFFFCAAF
		pkt, err := u.ReadPacket()
		if err != nil {
			return err
		}

		if got, want := pkt.Cmd, AppAck; got != want {
			return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
		}
	}

	// on the wire: FD000D010A0A40C2A8000102030405068B17
	removeAESPayload := make([]byte, 0, 3+channels)
	removeAESPayload = append(removeAESPayload, addr...)
	for i := 0; i < channels; i++ {
		removeAESPayload = append(removeAESPayload, byte(i))
	}
	if err := u.WritePacket(&Packet{
		Dst:     App,
		Cmd:     AppPeerRemoveAES,
		Payload: removeAESPayload,
	}); err != nil {
		return err
	}

	// on the wire: FD0004010A04010DB6
	pkt, err := u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	// Add peer / get peer config
	// on the wire: FD000901080640C2A8000000022E
	addPeerPayload := [6]byte{
		addr[0], addr[1], addr[2],
		0x00, 0x00, 0x00,
	}
	if err := u.WritePacket(&Packet{
		Dst:     App,
		Cmd:     AppAddPeer,
		Payload: addPeerPayload[:],
	}); err != nil {
		return err
	}

	// on the wire: FD0010010B040701010001FFFFFFFFFFFFFFFFC9A5
	pkt, err = u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	// Set key index + wakeup
	// on the wire: FD0009010C0640C2A80000004236
	addPeerPayload = [6]byte{
		addr[0], addr[1], addr[2],
		0x00, // key index 0, i.e. no encryption
		0x00, // don’t wake up
		0x00,
	}
	if err := u.WritePacket(&Packet{
		Dst:     App,
		Cmd:     AppAddPeer,
		Payload: addPeerPayload[:],
	}); err != nil {
		return err
	}

	// on the wire: FD0010010C040701010001FFFFFFFFFFFFFFFFCEB7
	pkt, err = u.ReadPacket()
	if err != nil {
		return err
	}

	if got, want := pkt.Cmd, AppAck; got != want {
		return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}

	return nil
}

func (u *UARTGW) Confirm() error {
	for {
		pkt, err := u.ReadPacket()
		if err != nil {
			return err
		}

		//log.Printf("pkt for confirmation: %+v", pkt)

		// TODO(later): verify messagecounter

		if pkt.Cmd == AppRecv {
			log.Printf("dropping pkt=%+v, looking for confirmation", pkt)
			continue
		}

		if got, want := pkt.Cmd, AppAck; got != want {
			return fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
		}

		break
	}
	return nil
}

func (u *UARTGW) AppSend(payload []byte) error {
	return u.WritePacket(&Packet{
		Dst:     App,
		Cmd:     AppSend,
		Payload: payload,
	})
}

// Write implements io.Writer so that a UARTGW can be used by the
// bidcos package.
func (u *UARTGW) Write(p []byte) (n int, err error) {
	return len(p), u.AppSend(p)
}

// Write implements io.Reader so that a UARTGW can be used by the
// bidcos package.
func (u *UARTGW) Read(p []byte) (n int, err error) {
	pkt, err := u.ReadPacket()
	if err != nil {
		return 0, err
	}
	if got, want := pkt.Cmd, AppRecv; got != want {
		return 0, fmt.Errorf("unexpected UARTGW packet cmd: got %v, want %v", got, want)
	}
	if got, want := len(p), len(pkt.Payload); got < want {
		return 0, fmt.Errorf("buffer too short for packet: got %v, want >= %v", got, want)
	}
	return copy(p, pkt.Payload), nil
}
