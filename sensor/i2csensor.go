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

	Crc8Polynomial byte = 0x31
	Crc8Init       byte = 0xFF
	Crc8XorOut     byte = 0x00
	Crc8Check      byte = 0xF7

	DefaultI2CFsPath   string  = "/dev/i2c-1"
	DefaultI2CAddr     byte    = 0x53
	DefaultFrequency   float32 = 1000.0
	DefaultDelayMillis int     = 5
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
}

func (s *SGP30Sensor) Init() error {
	if err := s.startI2CConnection(); err != nil {
		s.logError(err.Error())
		return err
	}
	s.delay(10)

	_, err := s.readWordsUint(InitAirQuality, 0)
	return err
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

func (s *SGP30Sensor) startI2CConnection() error {
	if s.i2cConnection != nil {
		return fmt.Errorf("i2cconnection already started")
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

func (s *SGP30Sensor) readWords(command []byte, replySize int) (result []uint16, err error) {
	if s.i2cConnection == nil {
		return nil, fmt.Errorf("i2c not connected")
	}

	err = s.i2cConnection.Write(command)
	if err != nil {
		s.logError("failed writing command %s: %s", hex.Dump(command), err)
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
			s.logError("crc mismatch %s, %s", crc, generatedCrc)
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

func (s *SGP30Sensor) logInfo(msg string) {
	if s.cfg.Logger != nil {
		s.cfg.Logger.Info(msg)
	}
}

func (s *SGP30Sensor) logDebug(msg string, params ...interface{}) {
	if s.cfg.Logger != nil {
		s.cfg.Logger.Debugf(msg, params)
	}
}
