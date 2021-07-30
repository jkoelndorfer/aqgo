package iotco1000

// This package controls the IOT-CO-1000 carbon monoxide sensor module
// via serial interface.
//
// See https://www.spec-sensors.com/product/iot-co-1000-digital-co-sensor-module/

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/tarm/serial"
)

type IOTCO1000 struct {
	SerialPort io.ReadWriteCloser
}

type AirQualityMeasurement struct {
	SensorSerialNumber string
	COConcentrationPPB int
	TemperatureC       int
	RelativeHumidity   int
	Uptime             time.Duration
	MeasurementTime    time.Time
}

func New(serialDevicePath string) (*IOTCO1000, error) {
	config := &serial.Config{
		Name:        serialDevicePath,
		Baud:        9600,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
		ReadTimeout: 250 * time.Millisecond,
	}
	serialPort, err := serial.OpenPort(config)
	if err != nil {
		return nil, err
	}
	iotco1000 := &IOTCO1000{
		SerialPort: serialPort,
	}
	return iotco1000, nil
}

func (co *IOTCO1000) Close() error {
	return co.SerialPort.Close()
}

func (co *IOTCO1000) AnalyzeAirQuality() (*AirQualityMeasurement, error) {
	bytesWritten, err := co.SerialPort.Write([]byte("\r\n"))
	if err != nil {
		return nil, err
	} else if bytesWritten == 0 {
		return nil, errors.New("failed to write to IOTCO1000 serial device")
	}

	// Give the IOTCO1000 a little bit of time to produce a response.
	time.Sleep(1000 * time.Millisecond)

	byteBuffer := make([]byte, 256)
	measurementTime := time.Now()
	totalBytesRead := 0
	for {
		bytesRead, err := co.SerialPort.Read(byteBuffer)
		totalBytesRead += bytesRead
		if err != nil {
			fmt.Printf("error is: %s", err)
			return nil, err
		}
		if totalBytesRead == 0 {
			// do nothing
		} else if byteBuffer[totalBytesRead-1] == byte('\n') {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	d := strings.Split(string(byteBuffer), ", ")
	serialNumber, COConcentrationPPB, temperatureC, relativeHumidity, daysUp, hoursUp, minutesUp, secondsUp :=
		d[0], d[1], d[2], d[3], d[7], d[8], d[9], d[10]

	COInt, err := strconv.ParseInt(COConcentrationPPB, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed converting CO concentration (%s) to int", COConcentrationPPB)
	}
	temperatureCInt, err := strconv.ParseInt(temperatureC, 10, 8)
	if err != nil {
		return nil, fmt.Errorf("failed converting temperature (%s) to int", temperatureC)
	}
	relativeHumidityInt, err := strconv.ParseInt(relativeHumidity, 10, 8)
	if err != nil {
		return nil, fmt.Errorf("failed converting relative humidity (%s) to int", relativeHumidity)
	}
	daysUpInt, err := strconv.ParseInt(daysUp, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed converting days up (%s) to int", daysUp)
	}
	hoursUpInt, err := strconv.ParseInt(hoursUp, 10, 8)
	if err != nil {
		return nil, fmt.Errorf("failed converting hours up (%s) to int", hoursUp)
	}
	uptimeDurationStr := fmt.Sprintf("%dh%sm%ss", daysUpInt*24+hoursUpInt, minutesUp, strings.Trim(secondsUp, " \r\n\x00"))
	uptime, err := time.ParseDuration(uptimeDurationStr)
	if err != nil {
		return nil, fmt.Errorf("failed parsing duration string %s", uptimeDurationStr)
	}

	return &AirQualityMeasurement{
		SensorSerialNumber: serialNumber,
		COConcentrationPPB: int(COInt),
		TemperatureC:       int(temperatureCInt),
		RelativeHumidity:   int(relativeHumidityInt),
		Uptime:             uptime,
		MeasurementTime:    measurementTime,
	}, nil
}
