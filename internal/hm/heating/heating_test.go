package heating_test

import (
	"fmt"
	"testing"

	"github.com/stapelberg/homematic/internal/bidcos"
	"github.com/stapelberg/homematic/internal/hm"
	"github.com/stapelberg/homematic/internal/hm/heating"
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

func TestDecodeInfoEvent(t *testing.T) {
	gw := testGateway{}
	bcs, err := bidcos.NewSender(&gw, [3]byte{0xfd, 0xee, 0xdd})
	if err != nil {
		t.Fatal(err)
	}
	ts := heating.NewThermostat(hm.StandardDevice{BCS: bcs, Addr: [3]byte{0xaa, 0xbb, 0xcc}})
	ie, err := ts.DecodeInfoEvent([]byte{0x0a, 0xb0, 0xe2, 0x08, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("info event: %+v", ie)
	if got, want := ie.SetTemperature, 22.0; got != want {
		t.Fatalf("unexpected set temperature: got %v, want %v", got, want)
	}
	if got, want := ie.ActualTemperature, 22.6; got != want {
		t.Fatalf("unexpected actual temperature: got %v, want %v", got, want)
	}
	if got, want := ie.BatteryState, 2.3; got != want {
		t.Fatalf("unexpected battery state: got %v, want %v", got, want)
	}
}
