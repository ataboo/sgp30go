package main

import (
	"time"

	"github.com/ataboo/sgp30go/sensor"
	"github.com/op/go-logging"
)

func main() {
	logger := logging.MustGetLogger("sgp30-console")
	cfg := sensor.DefaultConfig()
	cfg.Logger = logger
	sensor := sensor.NewSensor(cfg)

	if err := sensor.Init(); err != nil {
		panic(err)
	}
	defer sensor.Close()

	logger.Info("Connected to sensor with serial: %d", sensor.SerialID)

	if err := sensor.SetBaseline(0x8973, 0x8aae); err != nil {
		logger.Error("failed to set baseline", err)
	}

	for {
		select {
		case <-time.Tick(time.Second):
			eCO2, TVOC, err := sensor.Measure()
			if err != nil {
				logger.Error("failed to measure", err)
			} else {
				logger.Infof("Measurement: eCO2 - %x, TVOC - %x", eCO2, TVOC)
			}
		case <-time.Tick(time.Second * 10):
			eCo2Base, TVOCBase, err := sensor.GetBaseline()
			if err != nil {
				logger.Error("failed to get base", err)
			} else {
				logger.Infof("Baseline: eCO2 - %x, TVOC - %x", eCo2Base, TVOCBase)
			}

			if err := sensor.SetBaseline(eCo2Base, TVOCBase); err != nil {
				logger.Error("failed to set base", err)
			}
		}
	}
}
