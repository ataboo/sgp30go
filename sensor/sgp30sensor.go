package sensor

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/op/go-logging"
	"github.com/sigurn/crc8"
	"golang.org/x/exp/io/i2c"
)

const (
	InitAirQuality       uint16 = 0x2003
	MeasureAirQuality    uint16 = 0x2008
	GetBaseline          uint16 = 0x2015
	SetBaseline          uint16 = 0x201e
	SetHumidity          uint16 = 0x2061
	MeasureTest          uint16 = 0x2032
	GetFeatureSetVersion uint16 = 0x202f
	MeasureRawSignals    uint16 = 0x2050
	GetSerialID          uint16 = 0x3682
	ExpectedFeatureSet   uint16 = 0x0020

	Crc8Polynomial byte = 0x31
	Crc8Init       byte = 0xFF
	Crc8XorOut     byte = 0x00
	Crc8Check      byte = 0xF7

	DefaultI2CFsPath   string  = "/dev/i2c-1"
	DefaultI2CAddr     byte    = 0x58
	DefaultFrequency   float32 = 100000.0
	DefaultDelayMillis int     = 10
)

type i2CConnection interface {
	Read(buf []byte) error
	ReadReg(reg byte, buf []byte) error
	Write(buf []byte) error
	WriteReg(reg byte, buf []byte) (err error)
	Close() error
}

type Config struct {
	I2CFsPath   string
	I2CAddr     byte
	Frequency   float32
	Logger      *logging.Logger
	DelayMillis int
}

func DefaultConfig() *Config {
	return &Config{
		I2CFsPath:   DefaultI2CFsPath,
		I2CAddr:     DefaultI2CAddr,
		Frequency:   DefaultFrequency,
		Logger:      nil,
		DelayMillis: DefaultDelayMillis,
	}
}

func NewSensor(cfg *Config) *SGP30Sensor {
	return &SGP30Sensor{
		cfg: cfg,
		crcTable: crc8.MakeTable(crc8.Params{
			Poly:   Crc8Polynomial,
			Init:   Crc8Init,
			RefIn:  false,
			RefOut: false,
			XorOut: Crc8XorOut,
			Check:  Crc8Check,
		}),
	}
}

type SGP30Sensor struct {
	cfg           *Config
	i2cConnection i2CConnection
	crcTable      *crc8.Table
	SerialID      uint64
}

func (s *SGP30Sensor) Init() error {
	if err := s.startI2CConnection(); err != nil {
		s.logError(err.Error())
		return err
	}
	s.delay(s.cfg.DelayMillis)

	if serial, err := s.getSerial(); err == nil {
		s.SerialID = serial
	} else {
		s.SerialID = 0
		s.logError("failed to get serial: %s", err)
	}

	if featureSet, err := s.getFeatureSet(); err == nil {
		if featureSet != ExpectedFeatureSet {
			s.logError("sgp30 featureset mismatch: %x", featureSet)
			return fmt.Errorf("sgp30 sensor not found")
		}
	} else {
		s.logError("failed to get feature set")
		return fmt.Errorf("sgp30 sensor not found")
	}

	if _, err := s.readWordsUint(InitAirQuality, 0); err != nil {
		return err
	}

	return nil
}

func (s *SGP30Sensor) Close() error {
	if s.i2cConnection == nil {
		return fmt.Errorf("connection already closed")
	}

	err := s.i2cConnection.Close()
	s.i2cConnection = nil

	return err
}

func (s *SGP30Sensor) Measure() (eCO2 uint16, TVOC uint16, err error) {
	vals, err := s.readWordsUint(MeasureAirQuality, 2)
	if err != nil {
		return 0, 0, err
	}

	return vals[0], vals[1], err
}

func (s *SGP30Sensor) GetBaseline() (eCO2 uint16, TVOC uint16, err error) {
	vals, err := s.readWordsUint(GetBaseline, 2)
	if err != nil {
		return 0, 0, err
	}

	return vals[0], vals[1], nil
}

func (s *SGP30Sensor) SetBaseline(eCO2 uint16, TVOC uint16) error {
	buffer := make([]byte, 2)
	binary.BigEndian.PutUint16(buffer, SetBaseline)

	buffer = append(buffer, s.packWordCrc(eCO2)...)
	buffer = append(buffer, s.packWordCrc(TVOC)...)

	_, err := s.readWords(buffer, 0)

	return err
}

func (s *SGP30Sensor) getSerial() (uint64, error) {
	vals, err := s.readWordsUint(GetSerialID, 3)
	if err != nil {
		return 0, fmt.Errorf("failed to read serial: %s", err)
	}

	return s.combineWords(vals), nil
}

func (s *SGP30Sensor) getFeatureSet() (uint16, error) {
	vals, err := s.readWordsUint(GetFeatureSetVersion, 1)
	if err != nil {
		return 0, fmt.Errorf("failed to get feature set: %s", err)
	}

	return vals[0], nil
}

func (s *SGP30Sensor) startI2CConnection() error {
	if s.i2cConnection != nil {
		s.logError("i2cconnection already started")
		return nil
	}

	if _, err := os.Stat(s.cfg.I2CFsPath); err != nil {
		return fmt.Errorf("i2c FS path not found")
	}

	device, err := i2c.Open(&i2c.Devfs{Dev: s.cfg.I2CFsPath}, int(s.cfg.I2CAddr))
	s.i2cConnection = device

	return err
}

func (s *SGP30Sensor) packWordCrc(word uint16) []byte {
	buffer := make([]byte, 2)
	binary.BigEndian.PutUint16(buffer, word)
	buffer = append(buffer, s.generateCrc(buffer))

	return buffer
}

func (s *SGP30Sensor) readWordsUint(command uint16, replySize int) (result []uint16, err error) {
	buffer := make([]byte, 2)
	binary.BigEndian.PutUint16(buffer, command)

	return s.readWords(buffer, replySize)
}

func (s *SGP30Sensor) combineWords(words []uint16) uint64 {
	combined := make([]byte, 8)

	for i := range words {
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, words[len(words)-1-i])
		combined[7-2*i] = buf[1]
		combined[7-(2*i+1)] = buf[0]
	}

	return binary.BigEndian.Uint64(combined)
}

func (s *SGP30Sensor) readWords(command []byte, replySize int) (result []uint16, err error) {
	if s.i2cConnection == nil {
		return nil, fmt.Errorf("i2c not connected")
	}

	err = s.i2cConnection.Write(command)
	if err != nil {
		s.logError("failed writing command %s: %s", hex.Dump(command), err.Error())
		return result, err
	}

	s.delay(s.cfg.DelayMillis)
	if replySize == 0 {
		return result, nil
	}

	crcResult := make([]byte, replySize*(3))
	err = s.i2cConnection.Read(crcResult)
	if err != nil {
		s.logError("failed read: %s", err)
		return result, err
	}

	result = make([]uint16, replySize)

	for i := 0; i < replySize; i++ {
		word := []byte{crcResult[3*i], crcResult[3*i+1]}
		crc := crcResult[3*i+2]

		generatedCrc := s.generateCrc(word)
		if generatedCrc != crc {
			s.logError("crc mismatch %+v, %+v", crc, generatedCrc)
			return nil, fmt.Errorf("crc mismatch %x, %x", crc, generatedCrc)
		}

		result[i] = binary.BigEndian.Uint16([]byte{word[0], word[1]})
	}

	return result, nil
}

func (s *SGP30Sensor) generateCrc(data []byte) byte {
	return crc8.Checksum(data, s.crcTable)
}

func (s *SGP30Sensor) delay(delayMillis int) {
	time.Sleep(time.Millisecond * time.Duration(delayMillis))
}

func (s *SGP30Sensor) logError(msg string, params ...interface{}) {
	if s.cfg.Logger != nil {
		s.cfg.Logger.Errorf(msg, params)
	}
}
