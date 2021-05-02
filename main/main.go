package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/jkoelndorfer/aqgo/iotco1000"
)

var CO_CONCENTRATION_PPB = "COConcentrationPPB"
var TEMPERATURE_C = "TemperatureC"
var RELATIVE_HUMIDITY = "RelativeHumidity"
var UPTIME = "Uptime"
var SENSOR_SERIAL_NUMBER = "SensorSerialNumber"

type ApplicationArguments struct {
	PollInterval     int
	MetricNamespace  string
	SerialDevicePath string
}

func main() {
	logger := log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile)
	args, err := parseArguments()
	if err != nil {
		logger.Fatal(err)
	}

	cw, err := newCloudWatchClient()
	if err != nil {
		logger.Fatal("failed creating CloudWatch client")
	}

	sensor, err := iotco1000.New(args.SerialDevicePath)
	if err != nil {
		logger.Fatal(err)
	}
	defer sensor.Close()

	ch := make(chan *iotco1000.AirQualityMeasurement)
	go submitMetricsToCloudWatch(logger, cw, args.MetricNamespace, ch)
	for {
		aq, err := sensor.AnalyzeAirQuality()
		if err != nil {
			logger.Println(err)
		} else {
			ch <- aq
		}
		time.Sleep(time.Duration(args.PollInterval) * time.Millisecond)
	}
}

func submitMetricsToCloudWatch(logger *log.Logger, cw *cloudwatch.Client, ns string, ch chan *iotco1000.AirQualityMeasurement) {
	loggedAboutUptime := false
	warmUpDuration := time.Hour * 2
	for {
		aq := <-ch
		if aq.Uptime < warmUpDuration {
			if !loggedAboutUptime {
				// sensor readings made when the IOTCO1000 sensor has recently powered on are not accurate
				logger.Printf("skipping metric submission because sensor has not been active for warm up duration %s\n", warmUpDuration)
				loggedAboutUptime = true
			}
			continue
		}
		dimensions := []cwtypes.Dimension{
			{
				Name:  &SENSOR_SERIAL_NUMBER,
				Value: &aq.SensorSerialNumber,
			},
		}
		coPPB := float64(aq.COConcentrationPPB)
		if coPPB < 0 {
			coPPB = 0
		}
		storageResolution := int32(1)
		params := &cloudwatch.PutMetricDataInput{
			Namespace: &ns,
			MetricData: []cwtypes.MetricDatum{
				{
					MetricName:        &CO_CONCENTRATION_PPB,
					Value:             &coPPB,
					Dimensions:        dimensions,
					Unit:              cwtypes.StandardUnitNone,
					StorageResolution: &storageResolution,
				},
				{
					MetricName:        &TEMPERATURE_C,
					Value:             ifp(aq.TemperatureC),
					Dimensions:        dimensions,
					Unit:              cwtypes.StandardUnitNone,
					StorageResolution: &storageResolution,
				},
				{
					MetricName:        &RELATIVE_HUMIDITY,
					Value:             ifp(aq.RelativeHumidity),
					Dimensions:        dimensions,
					Unit:              cwtypes.StandardUnitNone,
					StorageResolution: &storageResolution,
				},
				{
					MetricName:        &UPTIME,
					Value:             ffp(aq.Uptime.Seconds()),
					Dimensions:        dimensions,
					Unit:              cwtypes.StandardUnitSeconds,
					StorageResolution: &storageResolution,
				},
			},
		}
		_, err := cw.PutMetricData(context.TODO(), params)
		if err != nil {
			logger.Printf("error submitting metric data to cloudwatch: %s\n", err)
		}
	}
}

func newCloudWatchClient() (*cloudwatch.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("error loading AWS default config: %s", err)
	}
	cloudwatchClient := cloudwatch.NewFromConfig(cfg)
	return cloudwatchClient, nil
}

func parseArguments() (*ApplicationArguments, error) {
	args := ApplicationArguments{}
	pollInterval := flag.Int("poll-interval", 5000, "how frequently to poll for and submit readings, in millseconds")
	serialDevicePath := flag.String("serial-device-path", "", "the location of the serial device to poll for readings")
	metricNamespace := flag.String("metric-namespace", "", "the CloudWatch metric namespace for which to submit readings")
	flag.Parse()
	missingArguments := []string{}
	if *serialDevicePath == "" {
		missingArguments = append(missingArguments, "serial-device-path")
	}
	if *metricNamespace == "" {
		missingArguments = append(missingArguments, "metric-namespace")
	}
	if len(missingArguments) > 0 {
		return nil, errors.New(fmt.Sprint("missing required argument(s): ", strings.Join(missingArguments, ", ")))
	}
	args.PollInterval = *pollInterval
	args.SerialDevicePath = *serialDevicePath
	args.MetricNamespace = *metricNamespace
	return &args, nil
}

func ifp(i int) *float64 {
	f := float64(i)
	return &f
}

func ffp(i float64) *float64 {
	return &i
}
