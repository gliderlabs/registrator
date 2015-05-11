package sns

import (
	"encoding/json"
	"log"
	"net/url"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/sns"
	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	bridge.Register(new(Factory), "sns")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	if uri.Host == "" {
		log.Fatal("sns: ", uri)
	}

	return &SNSAdapter{
		svc:      sns.New(nil),
		topicArn: uri.Host,
	}
}

type SNSAdapter struct {
	svc      *sns.SNS
	topicArn string
}

func (r *SNSAdapter) publish(event string, service *bridge.Service) error {
	msg, err := json.MarshalIndent(service, "", "\t")
	if err != nil {
		return err
	}
	params := &sns.PublishInput{
		TopicARN: aws.String(r.topicArn),
		Message:  aws.String(string(msg)),
		MessageAttributes: &map[string]*sns.MessageAttributeValue{
			"DOCKER.EVENT": &sns.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(event),
			},
		},
	}
	resp, err := r.svc.Publish(params)
	if err != nil {
		return err
	}

	log.Println("Published to SNS:", *resp.MessageID)
	return nil
}

// Ping will fetch the SNS Topic attributes
func (r *SNSAdapter) Ping() error {
	return r.publish("PING", nil)
}

func (r *SNSAdapter) Register(service *bridge.Service) error {
	return r.publish("SERVICE_REGISTER", service)
}

func (r *SNSAdapter) Deregister(service *bridge.Service) error {
	return r.publish("SERVICE_DEREGISTER", service)
}

func (r *SNSAdapter) Refresh(service *bridge.Service) error {
	return r.publish("SERVICE_REFRESH", service)
}
