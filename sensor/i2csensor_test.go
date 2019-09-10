package sensor

import "testing"

func TestCrcGeneration(t *testing.T) {
	sensor := NewSensor(DefaultConfig())

	table := []struct {
		byteArray   []byte
		expectedCrc byte
	}{
		{[]byte{0x31}, 0x58},
		{[]byte{0x31, 0x32, 0x33, 0x34}, 0xCB},
		{[]byte("123456789"), Crc8Check},
	}

	for _, row := range table {
		crc := sensor.generateCrc(row.byteArray)
		if crc != row.expectedCrc {
			t.Error("crc mismatch", row.expectedCrc, crc)
		}
	}
}

func TestPackwordCrc(t *testing.T) {
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

func TestReadWordsCheckConnection(t *testing.T) {
	sensor := NewSensor(DefaultConfig())

	_, err := sensor.readWords(nil, 0)
	if err == nil {
		t.Error("expected error")
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
