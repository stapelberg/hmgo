package main

import (
	"encoding/binary"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/gokrazy/gokrazy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stapelberg/hmgo/internal/bidcos"
	"github.com/stapelberg/hmgo/internal/gpio"
	"github.com/stapelberg/hmgo/internal/hm"
	"github.com/stapelberg/hmgo/internal/hm/heating"
	"github.com/stapelberg/hmgo/internal/hm/power"
	"github.com/stapelberg/hmgo/internal/hm/thermal"
	"github.com/stapelberg/hmgo/internal/serial"
	"github.com/stapelberg/hmgo/internal/uartgw"
)

// prometheus metrics
var (
	lastContact = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "hm",
			Name:      "LastContact",
			Help:      "Last device contact as UNIX timestamps, i.e. seconds since the epoch",
		},
		[]string{"address", "name"})

	packetsDecoded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "hm",
			Name:      "PacketsDecoded",
			Help:      "number of BidCoS packets successfully decoded",
		},
		[]string{"type"})
)

func init() {
	prometheus.MustRegister(lastContact)
	prometheus.MustRegister(packetsDecoded)
}

// flags
var (
	serialPort = flag.String("serial_port",
		"/dev/ttyAMA0",
		"path to a serial port to communicate with the HM-MOD-RPI-PCB")

	listenAddress = flag.String("listen",
		":8013",
		"host:port to listen on")
)

var yearday = time.Now().YearDay()

func overrideWinter(program []thermal.Program) []thermal.Program {
	if yearday > 90 && yearday < 270 {
		return program // no change during summer
	}
	log.Printf("initial program: %+v", program)
	for i, prog := range program {
		for ii, entry := range prog.Endtimes {
			if entry.Endtime == uint64((17 * time.Hour).Minutes()) {
				prog.Endtimes[ii].Temperature = 25.0 // cannot be reached, i.e. heat permanently
			}
		}
		program[i] = prog
	}
	log.Printf("modified program: %+v", program)
	return program
}

func main() {
	flag.Parse()

	gokrazy.WaitForClock()

	// TODO(later): drop privileges (only need network + serial port)

	log.Printf("opening serial port %s", *serialPort)

	uart, err := os.OpenFile(*serialPort, os.O_EXCL|os.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0600)
	if err != nil {
		log.Fatal(err)
	}
	if err := serial.Configure(uintptr(uart.Fd())); err != nil {
		log.Fatal(err)
	}

	log.Printf("resetting HM-MOD-RPI-PCB via GPIO")

	// Reset the HM-MOD-RPI-PCB to ensure we are starting in a
	// known-good state.
	if err := gpio.ResetUARTGW(uintptr(uart.Fd())); err != nil {
		log.Fatal(err)
	}

	// Re-enable blocking syscalls, which are required by the Go
	// standard library.
	if err := syscall.SetNonblock(int(uart.Fd()), false); err != nil {
		log.Fatal(err)
	}

	hmid := [3]byte{0xfd, 0xb0, 0x2c}
	gw, err := uartgw.NewUARTGW(uart, hmid, time.Now())
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("initialized UARTGW %s (firmware %s)", gw.SerialNumber, gw.FirmwareVersion)

	// TODO(later): implement support for the UARTGW acknowledging commands as “pending”.

	// TODO(later): add deadline for reading a frame

	bcs, err := bidcos.NewSender(gw, hmid)
	if err != nil {
		log.Fatal(err)
	}

	// map from src addr to device
	byAddr := make(map[[3]byte]hm.Device)
	bySerial := make(map[string]hm.Device)

	// for convenience
	device := func(addr [3]byte, name string) hm.StandardDevice {
		return hm.StandardDevice{
			BCS:       bcs,
			Addr:      addr,
			HumanName: name,
		}
	}

	avr := power.NewPowerSwitch(device([3]byte{0x40, 0xc2, 0xa8}, "avr"))

	thermalBad := thermal.NewThermalControl(device([3]byte{0x39, 0x0f, 0x17}, "Bad"))
	thermalWohnzimmer := thermal.NewThermalControl(device([3]byte{0x39, 0x06, 0xeb}, "Wohnzimmer"))
	thermalSchlafzimmer := thermal.NewThermalControl(device([3]byte{0x39, 0x06, 0xda}, "Schlafzimmer"))
	thermalLea := thermal.NewThermalControl(device([3]byte{0x39, 0x0f, 0x27}, "Lea"))

	thermostatBad := heating.NewThermostat(device([3]byte{0x38, 0xe6, 0xe9}, "Bad"))
	thermostatWohnzimmer := heating.NewThermostat(device([3]byte{0x38, 0xf5, 0x9c}, "Wohnzimmer"))
	thermostatSchlafzimmer := heating.NewThermostat(device([3]byte{0x38, 0xe8, 0xe3}, "Schlafzimmer"))
	thermostatLea := heating.NewThermostat(device([3]byte{0x38, 0xe8, 0xef}, "Lea"))

	bySerial["MEQ0090662"] = thermalBad
	bySerial["MEQ0089016"] = thermalWohnzimmer
	bySerial["MEQ0088999"] = thermalSchlafzimmer
	bySerial["MEQ0090675"] = thermalLea

	bySerial["MEQ0059922"] = thermostatWohnzimmer
	bySerial["MEQ0058671"] = thermostatBad
	bySerial["MEQ0059220"] = thermostatSchlafzimmer
	bySerial["MEQ0059216"] = thermostatLea

	bySerial["MEQ1341845"] = avr

	byAddr[thermalBad.Addr] = thermalBad
	byAddr[thermalWohnzimmer.Addr] = thermalWohnzimmer
	byAddr[thermalSchlafzimmer.Addr] = thermalSchlafzimmer
	byAddr[thermalLea.Addr] = thermalLea

	byAddr[thermostatWohnzimmer.Addr] = thermostatWohnzimmer
	byAddr[thermostatBad.Addr] = thermostatBad
	byAddr[thermostatSchlafzimmer.Addr] = thermostatSchlafzimmer
	byAddr[thermostatLea.Addr] = thermostatLea

	byAddr[avr.Addr] = avr

	// Explicitly reset the prometheus metric for last contact so that
	// all devices have an entry.
	for _, dev := range byAddr {
		lastContact.With(prometheus.Labels{"name": dev.Name(), "address": dev.AddrHex()}).Set(0)
	}

	for addr, dev := range byAddr {
		log.Printf("adding peer %x", addr[:])
		if err := gw.AddPeer(addr[:], dev.Channels()); err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("reading program configuration of %v", thermalWohnzimmer)
	if err := thermalWohnzimmer.EnsureConfigured(0 /* channel */, 7 /* plist */, func(mem []byte) error {
		thermalWohnzimmer.SetPrograms(mem, overrideWinter([]thermal.Program{
			{
				DayMask: thermal.WeekdayMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((10 * time.Hour).Minutes()), 22.0},
					{ /* 10:00- */ uint64((17 * time.Hour).Minutes()), 17.0},
					{ /* 17:00- */ uint64((23 * time.Hour).Minutes()), 22.0},
				},
			},
			{
				DayMask: thermal.WeekendMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((23 * time.Hour).Minutes()), 22.0},
				},
			},
		}))
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	log.Printf("reading program configuration of %v", thermalBad)
	if err := thermalBad.EnsureConfigured(0 /* channel */, 7 /* plist */, func(mem []byte) error {
		thermalBad.SetPrograms(mem, overrideWinter([]thermal.Program{
			{
				DayMask: thermal.WeekdayMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((10 * time.Hour).Minutes()), 22.0},
					{ /* 10:00- */ uint64((17 * time.Hour).Minutes()), 17.0},
					{ /* 17:00- */ uint64((23 * time.Hour).Minutes()), 22.0},
				},
			},
			{
				DayMask: thermal.WeekendMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((23 * time.Hour).Minutes()), 22.0},
				},
			},
		}))
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	log.Printf("reading program configuration of %v", thermalSchlafzimmer)
	if err := thermalSchlafzimmer.EnsureConfigured(0 /* channel */, 7 /* plist */, func(mem []byte) error {
		thermalSchlafzimmer.SetPrograms(mem, overrideWinter([]thermal.Program{
			{
				DayMask: thermal.WeekdayMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((10 * time.Hour).Minutes()), 22.0},
					{ /* 10:00- */ uint64((17 * time.Hour).Minutes()), 17.0},
					{ /* 17:00- */ uint64((23 * time.Hour).Minutes()), 24.0},
				},
			},
			{
				DayMask: thermal.WeekendMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((23 * time.Hour).Minutes()), 24.0},
				},
			},
		}))
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	log.Printf("reading program configuration of %v", thermalLea)
	if err := thermalLea.EnsureConfigured(0 /* channel */, 7 /* plist */, func(mem []byte) error {
		thermalLea.SetPrograms(mem, overrideWinter([]thermal.Program{
			{
				DayMask: thermal.WeekdayMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((10 * time.Hour).Minutes()), 22.0},
					{ /* 10:00- */ uint64((17 * time.Hour).Minutes()), 17.0},
					{ /* 17:00- */ uint64((23 * time.Hour).Minutes()), 22.0},
				},
			},
			{
				DayMask: thermal.WeekendMask,
				Endtimes: [13]thermal.ProgramEntry{
					{ /* 00:00- */ uint64((6 * time.Hour).Minutes()), 17.0},
					{ /* 06:00- */ uint64((23 * time.Hour).Minutes()), 22.0},
				},
			},
		}))
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	log.Printf("ensuring %v is peered with %v", thermalWohnzimmer, thermostatWohnzimmer)
	if err := thermalWohnzimmer.EnsurePeeredWith(
		thermal.ThermalControlTransmit,
		hm.FullyQualifiedChannel{
			Peer:    thermostatWohnzimmer.Addr,
			Channel: heating.ClimateControlReceiver,
		}); err != nil {
		log.Fatal(err)
	}
	log.Printf("ensuring %v is peered with %v", thermostatWohnzimmer, thermalWohnzimmer)
	if err := thermostatWohnzimmer.EnsurePeeredWith(
		heating.ClimateControlReceiver,
		hm.FullyQualifiedChannel{
			Peer:    thermalWohnzimmer.Addr,
			Channel: thermal.ThermalControlTransmit,
		}); err != nil {
		log.Fatal(err)
	}

	log.Printf("ensuring %v is peered with %v", thermalBad, thermostatBad)
	if err := thermalBad.EnsurePeeredWith(
		thermal.ThermalControlTransmit,
		hm.FullyQualifiedChannel{
			Peer:    thermostatBad.Addr,
			Channel: heating.ClimateControlReceiver,
		}); err != nil {
		log.Fatal(err)
	}
	log.Printf("ensuring %v is peered with %v", thermostatBad, thermalBad)
	if err := thermostatBad.EnsurePeeredWith(
		heating.ClimateControlReceiver,
		hm.FullyQualifiedChannel{
			Peer:    thermalBad.Addr,
			Channel: thermal.ThermalControlTransmit,
		}); err != nil {
		log.Fatal(err)
	}

	log.Printf("ensuring %v is peered with %v", thermalSchlafzimmer, thermostatSchlafzimmer)
	if err := thermalSchlafzimmer.EnsurePeeredWith(
		thermal.ThermalControlTransmit,
		hm.FullyQualifiedChannel{
			Peer:    thermostatSchlafzimmer.Addr,
			Channel: heating.ClimateControlReceiver,
		}); err != nil {
		log.Fatal(err)
	}
	log.Printf("ensuring %v is peered with %v", thermostatSchlafzimmer, thermalSchlafzimmer)
	if err := thermostatSchlafzimmer.EnsurePeeredWith(
		heating.ClimateControlReceiver,
		hm.FullyQualifiedChannel{
			Peer:    thermalSchlafzimmer.Addr,
			Channel: thermal.ThermalControlTransmit,
		}); err != nil {
		log.Fatal(err)
	}

	log.Printf("ensuring %v is peered with %v", thermalLea, thermostatLea)
	if err := thermalLea.EnsurePeeredWith(
		thermal.ThermalControlTransmit,
		hm.FullyQualifiedChannel{
			Peer:    thermostatLea.Addr,
			Channel: heating.ClimateControlReceiver,
		}); err != nil {
		log.Fatal(err)
	}
	log.Printf("ensuring %v is peered with %v", thermostatLea, thermalLea)
	if err := thermostatLea.EnsurePeeredWith(
		heating.ClimateControlReceiver,
		hm.FullyQualifiedChannel{
			Peer:    thermalLea.Addr,
			Channel: thermal.ThermalControlTransmit,
		}); err != nil {
		log.Fatal(err)
	}

	log.Printf("power-cycling AVR")
	if err := avr.LevelSet(power.ChannelSwitch, power.Off, 0x00); err != nil {
		log.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	if err := avr.LevelSet(power.ChannelSwitch, power.On, 0x00); err != nil {
		log.Fatal(err)
	}

	log.Printf("entering BidCoS packet handling main loop")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { handleStatus(w, r, bySerial) })
	http.Handle("/metrics", prometheus.Handler())
	go http.ListenAndServe(*listenAddress, nil)

	t := time.Tick(1 * time.Hour)
	for {
		select {
		case <-t:
			if err := gw.SetTime(time.Now()); err != nil {
				log.Fatalf("setting time: %v", err)
			}
		default:
		}

		pkt, err := gw.ReadPacket()
		if err != nil {
			log.Fatal(err)
		}
		if got, want := pkt.Cmd, uartgw.AppRecv; got != want {
			log.Fatalf("unexpected uartgw command in packet %+v: got %v, want %v", pkt, got, want)
		}

		bpkt, err := bidcos.Decode(pkt.Payload)
		if err != nil {
			log.Printf("skipping invalid bidcos packet: %v", err)
			continue
		}

		dev, ok := byAddr[bpkt.Source]
		if !ok {
			log.Printf("ignoring packet from unknown device [BidCoS:%x]", bpkt.Source)
			continue
		}

		lastContact.With(prometheus.Labels{"name": dev.Name(), "address": dev.AddrHex()}).Set(float64(time.Now().Unix()))

		switch bpkt.Cmd {
		default:
			log.Printf("unhandled BidCoS command from %x: %v", bpkt.Source, bpkt.Cmd)

		case bidcos.WeatherEvent:
			switch d := dev.(type) {
			case *thermal.ThermalControl:
				// Decode the event to update the prometheus metrics.
				if _, err := d.DecodeWeatherEvent(bpkt.Payload); err != nil {
					log.Printf("decoding weather event packet from %v: %v", bpkt.Source, err)
					continue
				}

				packetsDecoded.With(prometheus.Labels{"type": "hmthermal_WeatherEvent"}).Inc()

			default:
				log.Printf("ignoring unexpected BidCoS thermal control packet from device %x", bpkt.Source)
			}

		case bidcos.ThermalControl:
			switch d := dev.(type) {
			case *thermal.ThermalControl:
				// Decode the event to update the prometheus metrics.
				if _, err := d.DecodeThermalControlEvent(bpkt.Payload); err != nil {
					log.Printf("decoding thermal control event packet from %v: %v", bpkt.Source, err)
					continue
				}

				packetsDecoded.With(prometheus.Labels{"type": "hmthermal_ThermalControlEvent"}).Inc()

			default:
				log.Printf("ignoring unexpected BidCoS thermal control event packet from device %x", bpkt.Source)
			}

		case bidcos.PowerEventCyclic:
			fallthrough
		case bidcos.PowerEvent:
			switch d := dev.(type) {
			case *power.PowerSwitch:
				// Decode the event to update the prometheus metrics.
				if _, err := d.DecodePowerEvent(bpkt.Payload); err != nil {
					log.Printf("decoding power event packet from %v: %v", bpkt.Source, err)
					continue
				}

				packetsDecoded.With(prometheus.Labels{"type": "hmpower_PowerEvent"}).Inc()

			default:
				log.Printf("ignoring unexpected BidCoS power event packet from device %x", bpkt.Source)
			}

		case bidcos.Info:
			switch d := dev.(type) {
			case *thermal.ThermalControl:
				if _, err := d.DecodeInfoEvent(bpkt.Payload); err != nil {
					log.Printf("decoding power event packet from %v: %v", bpkt.Source, err)
					continue
				}

				packetsDecoded.With(prometheus.Labels{"type": "hmthermal_InfoEvent"}).Inc()

			case *heating.Thermostat:
				if _, err := d.DecodeInfoEvent(bpkt.Payload); err != nil {
					log.Printf("decoding power event packet from %v: %v", bpkt.Source, err)
					continue
				}

				packetsDecoded.With(prometheus.Labels{"type": "hmheating_InfoEvent"}).Inc()

			default:
				log.Printf("ignoring unexpected BidCoS info packet from device %x", bpkt.Source)
			}

		case bidcos.DeviceInfo:
			// c.f. https://github.com/Homegear/Homegear-HomeMaticBidCoS/blob/5255288954f3da42e12fa72a06963b99089d323f/src/HomeMaticCentral.cpp#L2997
			// TODO: add PeeringRequest type and decode method
			log.Printf("configuring new peer")
			if got, want := len(bpkt.Payload), 13; got < want {
				log.Printf("unexpectedly short payload: got %d, want >= %d", got, want)
				continue
			}
			firmware := bpkt.Payload[0]
			typ := binary.BigEndian.Uint16(bpkt.Payload[1 : 1+2])
			serial := string(bpkt.Payload[3 : 3+10])
			// e.g. peer request (fw 18, typ 173, serial MEQ0089016)
			log.Printf("peer request (fw %x, typ %d, serial %s)", firmware, typ, serial)
			dev, ok := bySerial[serial]
			if !ok {
				log.Printf("serial %q not configured, not replying to peering request", serial)
				continue
			}

			if d, ok := byAddr[bpkt.Source]; !ok || d != dev {
				log.Printf("device with serial %q uses unconfigured BidCoS address %x, not replying to peering request", serial, bpkt.Source)
				continue
			}

			if err := gw.AddPeer(bpkt.Source[:], dev.Channels()); err != nil {
				log.Fatal(err)
			}

			log.Printf("peer added, starting config")
			if err := dev.Pair(); err != nil {
				log.Fatal(err)
			}
			log.Printf("config end")
		}
	}
}
