package thermal_test

import (
	"fmt"
	"testing"

	"github.com/stapelberg/hmgo/internal/bidcos"
	"github.com/stapelberg/hmgo/internal/hm"
	"github.com/stapelberg/hmgo/internal/hm/thermal"
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

func TestDecodeWeatherEvent(t *testing.T) {
	gw := testGateway{}
	bcs, err := bidcos.NewSender(&gw, [3]byte{0xfd, 0xee, 0xdd})
	if err != nil {
		t.Fatal(err)
	}
	tc := thermal.NewThermalControl(hm.StandardDevice{BCS: bcs, Addr: [3]byte{0xaa, 0xbb, 0xcc}})
	we, err := tc.DecodeWeatherEvent([]byte{0, 253, 57})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := we.Temperature, 25.3; got != want {
		t.Fatalf("unexpected temperature: got %v, want %v", got, want)
	}
	if got, want := we.Humidity, uint64(57); got != want {
		t.Fatalf("unexpected humidity: got %v, want %v", got, want)
	}
}

func TestDecodeThermalControlEvent(t *testing.T) {
	gw := testGateway{}
	bcs, err := bidcos.NewSender(&gw, [3]byte{0xfd, 0xee, 0xdd})
	if err != nil {
		t.Fatal(err)
	}
	tc := thermal.NewThermalControl(hm.StandardDevice{BCS: bcs, Addr: [3]byte{0xaa, 0xbb, 0xcc}})
	tce, err := tc.DecodeThermalControlEvent([]byte{200, 215, 65})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tce.SetTemperature, 25.0; got != want {
		t.Fatalf("unexpected set temperature: got %v, want %v", got, want)
	}
	if got, want := tce.ActualTemperature, 21.5; got != want {
		t.Fatalf("unexpected actual temperature: got %v, want %v", got, want)
	}
	if got, want := tce.ActualHumidity, 65.0; got != want {
		t.Fatalf("unexpected humidity: got %v, want %v", got, want)
	}
}

func TestDecodeInfoEvent(t *testing.T) {
	gw := testGateway{}
	bcs, err := bidcos.NewSender(&gw, [3]byte{0xfd, 0xee, 0xdd})
	if err != nil {
		t.Fatal(err)
	}
	tc := thermal.NewThermalControl(hm.StandardDevice{BCS: bcs, Addr: [3]byte{0xaa, 0xbb, 0xcc}})
	ie, err := tc.DecodeInfoEvent([]byte{0x0b, 0xb0, 0xdf, 0x0e, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("info event: %+v", ie)
	if got, want := ie.SetTemperature, 22.0; got != want {
		t.Fatalf("unexpected set temperature: got %v, want %v", got, want)
	}
	if got, want := ie.ActualTemperature, 22.3; got != want {
		t.Fatalf("unexpected actual temperature: got %v, want %v", got, want)
	}
	if got, want := ie.BatteryState, 2.9; got != want {
		t.Fatalf("unexpected battery state: got %v, want %v", got, want)
	}
}
