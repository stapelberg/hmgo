package power_test

import (
	"fmt"
	"testing"

	"github.com/stapelberg/homematic/internal/bidcos"
	"github.com/stapelberg/homematic/internal/hm"
	"github.com/stapelberg/homematic/internal/hm/power"
)

type testGateway struct {
}

func (t *testGateway) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("reading not supported")
}

func (t *testGateway) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("writing not supported")
}

func (t *testGateway) Confirm() error {
	return nil
}

func TestDecodePowerEvent(t *testing.T) {
	gw := testGateway{}
	bcs, err := bidcos.NewSender(&gw, [3]byte{0xfd, 0xee, 0xdd})
	if err != nil {
		t.Fatal(err)
	}
	ps := power.NewPowerSwitch(hm.StandardDevice{BCS: bcs, Addr: [3]byte{0xaa, 0xbb, 0xcc}})

	// payload captured measuring a Raspberry Pi 3 :)
	pe, err := ps.DecodePowerEvent([]byte{128, 3, 138, 0, 0, 187, 0, 16, 9, 8, 255})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := pe.Boot, true; got != want {
		t.Fatalf("unexpected boot: got %v, want %v", got, want)
	}
	if got, want := pe.EnergyCounter, 90.6; got != want {
		t.Fatalf("unexpected energy counter: got %v, want %v", got, want)
	}
	if got, want := pe.Power, 1.87; got != want {
		t.Fatalf("unexpected power: got %v, want %v", got, want)
	}
	if got, want := pe.Current, 16.0; got != want {
		t.Fatalf("unexpected current: got %v, want %v", got, want)
	}
	if got, want := pe.Voltage, 231.2; got != want {
		t.Fatalf("unexpected voltage: got %v, want %v", got, want)
	}
	if got, want := pe.Frequency, 52.55; got != want {
		t.Fatalf("unexpected frequency: got %v, want %v", got, want)
	}
}
