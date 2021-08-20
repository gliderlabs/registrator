package influxdb

import (
	"context"
	"log"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go"
)

type Metrics struct {
	ServiceName   string
	ContainerID   string
	HostName      string
	ServicePort   int
	ServiceIP     string
	ServiceStatus string
	ServiceTags   []string
}

///////////////////////

type InfluxDBClient struct {
	BucketName  string
	InfluxToken string
	InfluxdbURL string
	client      influxdb2.Client
}

func New() InfluxDBClient {
	return InfluxDBClient{
		BucketName: os.Getenv("bucket"),
		client:     influxdb2.NewClient(os.Getenv("influx_url"), os.Getenv("influx_token")),
	}
}

func (c *InfluxDBClient) WriteData(metrics *Metrics) {

	client := c.client
	defer client.Close()
	// Use blocking write client for writes to desired bucket

	writeAPI := client.WriteAPIBlocking("wkda", c.BucketName)
	// write some points

	p := influxdb2.NewPointWithMeasurement("stat").
		AddTag("service_name", metrics.ServiceName).
		AddField("container_id", metrics.ContainerID).
		AddField("host", metrics.HostName).
		AddField("port", metrics.ServicePort).
		AddField("ip", metrics.ServiceIP).
		AddField("status", metrics.ServiceStatus).
		AddField("tags", metrics.ServiceTags).
		SetTime(time.Now())

	// write synchronously
	err := writeAPI.WritePoint(context.Background(), p)
	if err != nil {
		log.Println("Error writing to influxdb. Error is: ", err)
	}
}
