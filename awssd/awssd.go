package awssd

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/gliderlabs/registrator/bridge"
)

var autoCreateNewServices bool = false
var operationTimeout int = 10

func init() {
	bridge.Register(new(Factory), "aws-sd")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	var create = strings.ToLower(os.Getenv("AUTOCREATE_SERVICES"))
	if create != "" && create != "0" {
		autoCreateNewServices = true
	}
	var timeout = strings.ToLower(os.Getenv("OPERATION_TIMEOUT"))
	if val, err := strconv.Atoi(timeout); err == nil {
		operationTimeout = val
	}

	cfg, err := external.LoadDefaultAWSConfig() //Assumes that services are registered from the same region as the credentials

	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	namespaceId := uri.Host

	if namespaceId == "" {
		log.Fatal("must provide namespaceId. e.g. aws-sd://namespaceId")
	}
	client := servicediscovery.New(cfg)
	return &AwsAdapter{
		client:      client,
		namespaceId: &namespaceId,
	}
}

type AwsAdapter struct {
	client      *servicediscovery.ServiceDiscovery
	namespaceId *string
}

func (r *AwsAdapter) Ping() error {
	inp := &servicediscovery.GetNamespaceInput{
		Id: r.namespaceId,
	}
	req := r.client.GetNamespaceRequest(inp)
	_, err := req.Send()
	if err != nil {
		return err
	}
	return nil
}

func (r *AwsAdapter) Register(service *bridge.Service) error {
	var svcId *string = nil
	services, err := findServicesByName(r, service.Name)
	if err != nil {
		return err
	}
	if len(services) > 1 {
		return errors.New(fmt.Sprintf("Found %d services for service \"%s\". Expected 1 or 0", len(services), service.Name))
	}
	if len(services) == 0 { //create service definition. Unsure about race conditions with multiple agents.
		if !autoCreateNewServices {
			return errors.New(
				fmt.Sprintf("Service \"%s\" does not exist in namespace \"%s\" and AUTOCREATE_SERVICES is not set to TRUE",
					service.Name, *r.namespaceId))
		}
		outp, err := createService(service, r)
		if err != nil {
			return err
		}
		svcId = outp.Service.Id
	} else {
		svcId = services[0].Id // there should be exactly one svc
	}

	instInp := &servicediscovery.RegisterInstanceInput{
		Attributes: map[string]string{
			"AWS_INSTANCE_IPV4": service.IP,
			"AWS_INSTANCE_PORT": strconv.Itoa(service.Port),
		},
		InstanceId: &service.ID,
		ServiceId:  svcId,
	}
	instReq := r.client.RegisterInstanceRequest(instInp)
	resp, err := instReq.Send()
	if err != nil {
		return err
	}
	err = getOperationResultSync(r.client, *resp.OperationId, "Register")
	if err != nil {
		return err
	}
	return nil
}

func createService(service *bridge.Service, adapter *AwsAdapter) (*servicediscovery.CreateServiceOutput, error) {
	var ttl int64 = int64(service.TTL)
	dnscfg := servicediscovery.DnsConfig{
		DnsRecords: []servicediscovery.DnsRecord{
			{
				TTL:  &ttl,
				Type: servicediscovery.RecordTypeSrv,
			},
		},
		NamespaceId: adapter.namespaceId,
	}

	inp := &servicediscovery.CreateServiceInput{
		DnsConfig: &dnscfg,
		Name:      &service.Name,
	}
	req := adapter.client.CreateServiceRequest(inp)
	return req.Send()
}

type OperationQueryResult struct {
	output *servicediscovery.GetOperationOutput
	error  error
}

// This is a bit nasty, would be nice to filter by name in the API. Not supported as at 2018-04-16
func findServicesByName(adapter *AwsAdapter, name string) ([]servicediscovery.ServiceSummary, error) {
	filters := []servicediscovery.ServiceFilter{
		{
			Condition: "EQ",
			Name:      servicediscovery.ServiceFilterNameNamespaceId,
			Values:    []string{*adapter.namespaceId},
		},
	}
	var nextToken *string = nil
	var services []servicediscovery.ServiceSummary
	for {
		inp := &servicediscovery.ListServicesInput{
			Filters:   filters,
			NextToken: nextToken, // generally expected to be nil
		}
		req := adapter.client.ListServicesRequest(inp)
		res, err := req.Send()
		if err != nil {
			return nil, err
		}

		for _, v := range res.Services {
			if *v.Name == name {
				services = append(services, v)
			}
		}
		nextToken = res.NextToken
		if nextToken == nil {
			break
		}
	}
	return services, nil
}

func (r *AwsAdapter) Deregister(service *bridge.Service) error {
	services, err := findServicesByName(r, service.Name)
	if err != nil {
		return err
	}
	if len(services) != 1 {
		return errors.New(fmt.Sprintf("Found %d services for service \"%s\". Expected 1", len(services), service.Name))
	}

	inp := &servicediscovery.DeregisterInstanceInput{
		InstanceId: &service.ID,
		ServiceId:  services[0].Id,
	}
	req := r.client.DeregisterInstanceRequest(inp)
	resp, err := req.Send()
	if err != nil {
		return err
	}
	err = getOperationResultSync(r.client, *resp.OperationId, "Deregister")
	if err != nil {
		return err
	}
	return nil
}

func getOperationResultSync(client *servicediscovery.ServiceDiscovery, operationId string, opType string) error {
	operationChannel := make(chan OperationQueryResult, 1)
	for {
		go getOperationResult(operationChannel, client, operationId)
		select {
		case res := <-operationChannel:
			if res.error != nil {
				return res.error
			}
			op := res.output.Operation
			if op.Status == "FAIL" {
				return errors.New(fmt.Sprintf("%s operation \"%s\" failed with: %s\n%s",
					opType, *op.Id, *op.ErrorCode, *op.ErrorMessage))
			} else if op.Status == "SUCCESS" {
				return nil
			} else {
				time.Sleep(1 * time.Second)
			}
		case <-time.After(time.Duration(operationTimeout) * time.Second):
			return errors.New(fmt.Sprintf("%s operation \"%s\" took more than %d seconds to respond with SUCCESS/FAIL",
				opType, operationId, operationTimeout))
		}
	}
}

func getOperationResult(done chan OperationQueryResult, client *servicediscovery.ServiceDiscovery, operationId string) {
	inp := servicediscovery.GetOperationInput{
		OperationId: &operationId,
	}
	req := client.GetOperationRequest(&inp)
	outp, err := req.Send()
	res := OperationQueryResult{
		output: outp,
		error:  err,
	}
	done <- res
}

func (r *AwsAdapter) Refresh(service *bridge.Service) error {
	return nil
}

func findServices(adapter *AwsAdapter) ([]servicediscovery.ServiceSummary, error) {
	var nextToken *string = nil
	var services []servicediscovery.ServiceSummary
	for {
		filters := []servicediscovery.ServiceFilter{
			{
				Condition: "EQ",
				Name:      servicediscovery.ServiceFilterNameNamespaceId,
				Values:    []string{*adapter.namespaceId},
			},
		}
		inp := servicediscovery.ListServicesInput{
			Filters: filters,
		}
		req := adapter.client.ListServicesRequest(&inp)
		resp, err := req.Send()
		if err != nil {
			return nil, err
		}
		services = append(services, resp.Services...)
		nextToken = resp.NextToken
		if nextToken == nil {
			break
		}
	}
	return services, nil
}

func (r *AwsAdapter) Services() ([]*bridge.Service, error) {
	services, err := findServices(r)
	if err != nil {
		return nil, err
	}
	var out []*bridge.Service
	for _, service := range services {
		var nextToken *string = nil
		for { // this uses break when the next token is null
			inp := servicediscovery.ListInstancesInput{
				ServiceId: service.Id,
				NextToken: nextToken,
			}
			req := r.client.ListInstancesRequest(&inp)
			resp, err := req.Send()
			if err != nil {
				return nil, err
			}
			for _, inst := range resp.Instances {
				port, err := strconv.Atoi(inst.Attributes["AWS_INSTANCE_PORT"])
				if err != nil {
					return nil, errors.New("failed to cast port to int")
				}

				s := &bridge.Service{
					ID:   *inst.Id,
					Name: *service.Name,
					Port: port,
					IP:   inst.Attributes["AWS_INSTANCE_IPV4"],
				}
				out = append(out, s)
			}
			nextToken = resp.NextToken
			if nextToken == nil {
				break
			}
		}
	}
	return out, nil
}
