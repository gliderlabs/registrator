package dynamodb

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gliderlabs/registrator/bridge"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	client *dynamodb.DynamoDB
	table  string
}

func (c *Client) Set(key, value string, ttl uint64) (*dynamodb.PutItemOutput, error) {
	created := time.Now()
	// log.Println("dynamodb: set key: " + key + ", value: " + value)
	return c.client.PutItem(&dynamodb.PutItemInput{
		Item: map[string]*dynamodb.AttributeValue{
			"key":     &dynamodb.AttributeValue{S: aws.String(key)},
			"value":   &dynamodb.AttributeValue{S: aws.String(value)},
			"created": &dynamodb.AttributeValue{N: aws.String(strconv.FormatInt(created.Unix(), 10))},
			"expired": &dynamodb.AttributeValue{N: aws.String(strconv.FormatInt(created.Unix()+int64(ttl), 10))},
		},
		TableName: &c.table})
}

func (c *Client) Delete(key string, isBool bool) (*dynamodb.DeleteItemOutput, error) {
	// log.Println("dynamodb: delete key: " + key)
	return c.client.DeleteItem(&dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"key": &dynamodb.AttributeValue{S: aws.String(key)},
		},
		TableName: &c.table})
}

func NewClient(table string) (*Client, error) {
	var c *aws.Config
	if os.Getenv("DYNAMODB_LOCAL") != "" {
		c = &aws.Config{Endpoint: os.Getenv("DYNAMODB_LOCAL")}
	} else {
		c = nil
	}

	d := dynamodb.New(c)
	// Check if the table exists
	_, err := d.DescribeTable(&dynamodb.DescribeTableInput{TableName: &table})
	if err != nil {
		return nil, err
	}

	return &Client{client: d, table: table}, nil
}

func init() {
	bridge.Register(new(Factory), "dynamodb")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	if len(uri.Path) < 2 {
		log.Fatal("dynamodb: table name required e.g.: dynamodb://<table>/<path>")
	}

	// log.Println("dynamodb: tables name is " + uri.Host)
	client, err := NewClient(uri.Host)
	if err != nil {
		log.Fatal(err)
	}
	return &DynamodbAdapter{
		client: client,
		path:   domainPath(uri.Path[1:])}
}

type DynamodbAdapter struct {
	client *Client
	path   string
}

func (r *DynamodbAdapter) Ping() error {
	return nil
}

func (r *DynamodbAdapter) Register(service *bridge.Service) error {
	port := strconv.Itoa(service.Port)
	record := service.IP + ":" + port
	_, err := r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	if err != nil {
		log.Println("dynamodb: failed to register service:", err)
	}
	return err
}

func (r *DynamodbAdapter) Deregister(service *bridge.Service) error {
	_, err := r.client.Delete(r.servicePath(service), false)
	if err != nil {
		log.Println("dynamodb: failed to register service:", err)
	}
	return err
}

func (r *DynamodbAdapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *DynamodbAdapter) servicePath(service *bridge.Service) string {
	return r.path + "/" + service.Name + "/" + service.ID
}

func domainPath(domain string) string {
	components := strings.Split(domain, ".")
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return "/" + strings.Join(components, "/")
}
