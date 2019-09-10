package sensor

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestCrcGeneration(t *testing.T) {
	sensor := NewSensor(DefaultConfig())

	table := []struct {
		byteArray   []byte
		expectedCrc byte
	}{
		{[]byte{0x31}, 0x58},
		{[]byte{0x31, 0x32, 0x33, 0x34}, 0xCB},
		{[]byte{0x01, 0x02}, 0x17},
		{[]byte{0x03, 0x04}, 0x68},
		{[]byte{0x05, 0x06}, 0x50},
		{[]byte{0x00, 0x20}, 0x7},
		{[]byte("123456789"), Crc8Check},
	}

	for _, row := range table {
		crc := sensor.generateCrc(row.byteArray)
		if crc != row.expectedCrc {
			t.Errorf("crc mismatch, %x, %x", row.expectedCrc, crc)
		}
	}
}

func TestPackWordCrc(t *testing.T) {
	sensor := NewSensor(DefaultConfig())
	table := []struct {
		word           uint16
		expectedPacked []byte
	}{
		{0x1234, []byte{0x12, 0x34, 0x37}},
		{0x00, []byte{0x00, 0x00, 0x81}},
	}

	for _, row := range table {
		packed := sensor.packWordCrc(row.word)
		if !_bytesMatch(packed, row.expectedPacked) {
			t.Error("mismatch packed", row.expectedPacked, packed)
		}
	}
}

func TestCombineWords(t *testing.T) {
	sensor := NewSensor(DefaultConfig())
	table := []struct {
		words            []uint16
		expectedCombined uint64
	}{
		{[]uint16{0x1234, 0x5678}, 0x12345678},
		{[]uint16{0x4321, 0x8765, 0xdcbe}, 0x43218765dcbe},
		{[]uint16{0x1234, 0x5678, 0x90ab, 0xcdef}, 0x1234567890abcdef},
	}

	for _, row := range table {
		combined := sensor.combineWords(row.words)
		if combined != row.expectedCombined {
			t.Errorf("mismatched result %x, %x", row.expectedCombined, combined)
		}
	}
}

func TestReadWordsChecksConnection(t *testing.T) {
	sensor := NewSensor(DefaultConfig())

	_, err := sensor.readWords(nil, 0)
	if err == nil {
		t.Error("expected error")
	}
}

func TestInit(t *testing.T) {
	mock := &_mockI2cConnection{}
	sensor := NewSensor(DefaultConfig())
	sensor.i2cConnection = mock

	mock.writeClosure = func(buf []byte) error {
		return fmt.Errorf("thrown error")
	}

	if err := sensor.Init(); err == nil {
		t.Error("expected error")
	}

	var readOutput []byte

	mock.writeClosure = func(buf []byte) error {
		if _bytesMatchUint(buf, InitAirQuality) {
			readOutput = nil
		} else if _bytesMatchUint(buf, GetSerialID) {
			readOutput = []byte{0x01, 0x02, 0x17, 0x03, 0x04, 0x68, 0x05, 0x06, 0x50}
		} else if _bytesMatchUint(buf, GetFeatureSetVersion) {
			readOutput = []byte{0x00, 0x20, 0x07}
		}

		return nil
	}

	mock.readClosure = func(buf []byte) error {
		if len(buf) != len(readOutput) {
			t.Error("output mismatch", len(readOutput), len(buf))
		}

		copy(buf, readOutput)

		return nil
	}

	if err := sensor.Init(); err != nil {
		t.Error("unexpected error")
	}

	if sensor.SerialID != 0x010203040506 {
		t.Error("unexpected serial id")
	}

	mock.writeClosure = func(buf []byte) error {
		if _bytesMatchUint(buf, InitAirQuality) {
			readOutput = nil
		} else if _bytesMatchUint(buf, GetSerialID) {
			readOutput = []byte{0x01, 0x02, 0x17, 0x03, 0x04, 0x68, 0x05, 0x06, 0x0}
		} else if _bytesMatchUint(buf, GetFeatureSetVersion) {
			readOutput = []byte{0x01, 0x02, 0x17}
		}

		return nil
	}

	if err := sensor.Init(); err == nil {
		t.Error("expected error")
	}

	if sensor.SerialID != 0 {
		t.Error("expected zeroed serial id")
	}

	mock.writeClosure = func(buf []byte) error {
		if _bytesMatchUint(buf, InitAirQuality) {
			readOutput = nil
		} else if _bytesMatchUint(buf, GetSerialID) {
			readOutput = []byte{0x01, 0x02, 0x17, 0x03, 0x04, 0x68, 0x05, 0x06, 0x0}
		} else if _bytesMatchUint(buf, GetFeatureSetVersion) {
			readOutput = []byte{0x01, 0x02, 0x00}
		}

		return nil
	}

	if err := sensor.Init(); err == nil {
		t.Error("expected error")
	}
}

func TestClose(t *testing.T) {
	sensor := NewSensor(DefaultConfig())
	if err := sensor.Close(); err == nil {
		t.Error("expected error")
	}

	closeCalled := false
	sensor.i2cConnection = &_mockI2cConnection{
		closeClosure: func() error {
			closeCalled = true
			return nil
		},
	}

	if err := sensor.Close(); err != nil {
		t.Error("unexpected error", err)
	}

	if !closeCalled {
		t.Error("cloase called should be set")
	}
}

func TestReadWords(t *testing.T) {
	mock := &_mockI2cConnection{}
	mock.writeClosure = func(buf []byte) error {
		if len(buf) != 1 {
			t.Fatal("unexpected buffer length", 1, len(buf))
		}

		if buf[0] != 0x23 {
			t.Error("unexpected buffer values", 0x23, buf[0])
		}

		return nil
	}
	mock.readClosure = func(buf []byte) error {
		if len(buf) != 3 {
			t.Fatal("unexpected buffer length", 3, len(buf))
		}

		buf[0] = 0x01
		buf[1] = 0x02
		buf[2] = 0x17

		return nil
	}

	sensor := NewSensor(DefaultConfig())
	sensor.cfg.DelayMillis = 0
	sensor.i2cConnection = mock

	val, err := sensor.readWords([]byte{0x23}, 1)
	if err != nil {
		t.Error("unexpected error", err)
	}

	if len(val) != 1 || val[0] != 0x0102 {
		t.Error("unexpected return value", 0x0102, val)
	}
}

func TestReadWordsHandlesErrors(t *testing.T) {
	mock := &_mockI2cConnection{}
	sensor := NewSensor(DefaultConfig())
	sensor.cfg.DelayMillis = 0
	sensor.i2cConnection = mock

	mock.writeClosure = func(buf []byte) error {
		return fmt.Errorf("write fail")
	}

	if _, err := sensor.readWords(nil, 1); err.Error() != "write fail" {
		t.Error("expected error")
	}

	mock.writeClosure = func(buf []byte) error {
		return nil
	}
	mock.readClosure = func(buf []byte) error {
		return fmt.Errorf("read fail")
	}

	if _, err := sensor.readWords(nil, 1); err.Error() != "read fail" {
		t.Error("expected error")
	}

	if _, err := sensor.readWords(nil, 0); err != nil {
		t.Error("unexpected error", err)
	}
}

func TestReadWordsHandlesCrcMismatch(t *testing.T) {
	mock := &_mockI2cConnection{}
	sensor := NewSensor(DefaultConfig())
	sensor.cfg.DelayMillis = 0
	sensor.i2cConnection = mock

	mock.readClosure = func(buf []byte) error {
		buf[0] = 0x01
		buf[1] = 0x02
		buf[2] = 0x03

		return nil
	}
	mock.writeClosure = func(buf []byte) error {
		return nil
	}

	if _, err := sensor.readWords(nil, 1); err == nil {
		t.Error("expected error")
	}
}

func TestMeasure(t *testing.T) {
	mock := &_mockI2cConnection{}
	sensor := NewSensor(DefaultConfig())
	sensor.cfg.DelayMillis = 0
	sensor.i2cConnection = mock

	mock.writeClosure = func(buf []byte) error {
		if len(buf) != 2 || buf[0] != 0x20 || buf[1] != 0x08 {
			t.Error("unexpected write value", 0x2008, buf)
		}

		return nil
	}

	mock.readClosure = func(buf []byte) error {
		if len(buf) != 6 {
			t.Fatal("unexpected read buffer length")
		}

		buf[0] = 0x01
		buf[1] = 0x02
		buf[2] = 0x17
		buf[3] = 0x03
		buf[4] = 0x04
		buf[5] = 0x68

		return nil
	}

	co2, tvoc, err := sensor.Measure()
	if err != nil {
		t.Error("unexpected error", err)
	}

	if co2 != 0x0102 {
		t.Errorf("unexpected co2 value, %x, %x", 0x0102, co2)
	}

	if tvoc != 0x0304 {
		t.Errorf("unexpected tvoc value, %x, %x", 0x0304, tvoc)
	}

	mock.readClosure = func(buf []byte) error {
		if len(buf) != 6 {
			t.Fatal("unexpected read buffer length")
		}

		buf[0] = 0x01
		buf[1] = 0x02
		buf[2] = 0x00
		buf[3] = 0x03
		buf[4] = 0x04
		buf[5] = 0x68

		return nil
	}

	if _, _, err := sensor.Measure(); err == nil {
		t.Error("expected error")
	}
}

func TestGetSerialNumber(t *testing.T) {
	mock := &_mockI2cConnection{}
	sensor := NewSensor(DefaultConfig())
	sensor.cfg.DelayMillis = 0
	sensor.i2cConnection = mock

	mock.writeClosure = func(buf []byte) error {
		if !_bytesMatch(buf, []byte{0x36, 0x82}) {
			t.Error("mismatched write buffer")
		}

		return nil
	}

	mock.readClosure = func(buf []byte) error {
		if len(buf) != 9 {
			t.Error("unexpected buffer len", 9, len(buf))
		}

		returnVal := []byte{0x01, 0x02, 0x17, 0x03, 0x04, 0x68, 0x05, 0x06, 0x50}

		copy(buf, returnVal)

		return nil
	}

	val, err := sensor.getSerial()
	if err != nil {
		t.Error("unexpected err", err)
	}

	if val != 0x010203040506 {
		t.Errorf("unexpected serial value, %x", val)
	}

	mock.writeClosure = func(buf []byte) error {
		return fmt.Errorf("error")
	}

	if _, err := sensor.getSerial(); err == nil {
		t.Error("expected error")
	}
}

func TestGetFeatureSet(t *testing.T) {

}

func TestGetBaseline(t *testing.T) {
	mock := &_mockI2cConnection{}
	sensor := NewSensor(DefaultConfig())
	sensor.cfg.DelayMillis = 0
	sensor.i2cConnection = mock

	mock.writeClosure = func(buf []byte) error {
		if len(buf) != 2 || buf[0] != 0x20 || buf[1] != 0x15 {
			t.Error("unexpected write value", 0x2015, buf)
		}

		return nil
	}

	mock.readClosure = func(buf []byte) error {
		if len(buf) != 6 {
			t.Fatal("unexpected read buffer length")
		}

		buf[0] = 0x01
		buf[1] = 0x02
		buf[2] = 0x17
		buf[3] = 0x03
		buf[4] = 0x04
		buf[5] = 0x68

		return nil
	}

	co2, tvoc, err := sensor.GetBaseline()
	if err != nil {
		t.Error("unexpected error", err)
	}

	if co2 != 0x0102 || tvoc != 0x0304 {
		t.Error("unexpected values")
	}

	mock.readClosure = func(buf []byte) error {
		if len(buf) != 6 {
			t.Fatal("unexpected read buffer length")
		}

		buf[0] = 0x01
		buf[1] = 0x02
		buf[2] = 0x17
		buf[3] = 0x03
		buf[4] = 0x04
		buf[5] = 0x00

		return nil
	}

	if _, _, err := sensor.GetBaseline(); err == nil {
		t.Error("expected error")
	}
}

func TestSetBaseline(t *testing.T) {
	mock := &_mockI2cConnection{}
	sensor := NewSensor(DefaultConfig())
	sensor.cfg.DelayMillis = 0
	sensor.i2cConnection = mock

	mock.writeClosure = func(buf []byte) error {
		if !_bytesMatch(buf, []byte{0x20, 0x1e, 0x01, 0x02, 0x17, 0x03, 0x04, 0x68}) {
			t.Error("unexpected buffer")
		}

		return nil
	}

	err := sensor.SetBaseline(0x0102, 0x0304)
	if err != nil {
		t.Error("unexpected error", err)
	}
}

func _bytesMatch(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func _bytesMatchUint(a []byte, intVal uint16) bool {
	return binary.BigEndian.Uint16(a) == intVal
}

type _mockI2cConnection struct {
	readClosure     func(buf []byte) error
	readRegClosure  func(reg byte, buf []byte) error
	writeClosure    func(buf []byte) error
	writeRegClosure func(reg byte, buf []byte) error
	closeClosure    func() error
}

func (m *_mockI2cConnection) Read(buf []byte) error {
	return m.readClosure(buf)
}

func (m *_mockI2cConnection) ReadReg(reg byte, buf []byte) error {
	return m.readRegClosure(reg, buf)
}

func (m *_mockI2cConnection) Write(buf []byte) error {
	return m.writeClosure(buf)
}

func (m *_mockI2cConnection) WriteReg(reg byte, buf []byte) (err error) {
	return m.writeRegClosure(reg, buf)
}

func (m *_mockI2cConnection) Close() error {
	return m.closeClosure()
}
