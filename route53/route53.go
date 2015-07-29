package route53

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/aws/awserr"
	r53 "github.com/awslabs/aws-sdk-go/service/route53"
	"github.com/gliderlabs/registrator/bridge"
)

const EC2MetaDataKey = "useEC2MetadataForHostname"

func init() {
	bridge.Register(new(Factory), "route53")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	// use ec2 metadata service
	q := uri.Query()

	useEc2Meatadata, err := strconv.ParseBool(q.Get(EC2MetaDataKey))
	if err != nil {
		useEc2Meatadata = false
	}

	// route53 zone ID
	zoneId := uri.Host

	if zoneId == "" {
		log.Fatal("must provide zoneId. e.g. route53://zoneId")
	}

	return &Route53Registry{client: r53.New(nil), path: uri.Path, useEc2Meatadata: useEc2Meatadata, zoneId: zoneId}
}

type Route53Registry struct {
	client          *r53.Route53
	path            string
	useEc2Meatadata bool
	zoneId          string
	dnsSuffix       string
}

// Ping gets the hosted zone name. This name will be used
// as a suffix to all DNS name entries
func (r *Route53Registry) Ping() error {
	params := &r53.GetHostedZoneInput{
		ID: aws.String(r.zoneId),
	}
	resp, err := r.client.GetHostedZone(params)

	r.dnsSuffix = *resp.HostedZone.Name
	return err
}

func (r *Route53Registry) Register(service *bridge.Service) error {
	// query Route53 for existing records
	name := service.Name + "." + r.dnsSuffix

	// determine the hostname
	hostname := r.getHostname()

	// append our new record and persist
	var recordSet ResourceRecordSet
	recordSet, err := r.GetServiceEntry(r.zoneId, name)

	if recordSet.nameIs(name) {
		// update existing DNS record
		value := fmt.Sprintf("1 10 %d %s", service.Port, hostname)
		log.Println("Updating DNS entry for", name, "adding values", value)
		// Since MaxItems is set to 1 we'll only ever get a single record
		// get the resource records associated with this name
		var resourceRecords ResourceRecords = recordSet[0].ResourceRecords

		resourceRecords = append(resourceRecords, &r53.ResourceRecord{Value: aws.String(value)})
		r.UpdateDns(r.zoneId, name, "UPSERT", resourceRecords)
	} else {
		// Create new DNS record
		value := fmt.Sprintf("1 10 %d %s", service.Port, hostname)
		log.Println("Creating new DNS Entry for", name, "with value", value)
		resourceRecord := []*r53.ResourceRecord{
			&r53.ResourceRecord{
				Value: aws.String(value),
			},
		}
		r.UpdateDns(r.zoneId, name, "UPSERT", resourceRecord)
	}

	return err
}

func (r *Route53Registry) Deregister(service *bridge.Service) error {

	// query Route53 for existing records
	name := service.Name + "." + r.dnsSuffix

	// determine the hostname
	hostname := r.getHostname()

	// Query Route 53 for for DNS record
	var recordSet ResourceRecordSet
	recordSet, err := r.GetServiceEntry(r.zoneId, name)

	if recordSet.nameIs(name) {
		// find the position of the value to deregister
		var resourceRecords ResourceRecords = recordSet[0].ResourceRecords
		pos := resourceRecords.pos(hostname)
		// remove record from set
		if pos != -1 {
			if len(resourceRecords) == 1 {
				// delete this DNS record set
				// the only associated value is the one we're removing
				r.UpdateDns(r.zoneId, name, "DELETE", resourceRecords)
			} else {
				// Remove the value referenced in the SRV record, do not remove the DNS entry
				resourceRecords = append(resourceRecords[:pos], resourceRecords[pos+1:]...)
				r.UpdateDns(r.zoneId, name, "UPSERT", resourceRecords)
			}
		}
	} else {
		log.Println("Could not find service", name, "to deregister")
	}

	return err
}

func (r *Route53Registry) Refresh(service *bridge.Service) error {
	return nil
}

// Gets route53 service entry for the provided zoneId and recordName
func (r *Route53Registry) GetServiceEntry(zoneId string, recordName string) ([]*r53.ResourceRecordSet, error) {
	params := &r53.ListResourceRecordSetsInput{
		HostedZoneID:    aws.String(zoneId),
		StartRecordName: aws.String(recordName),
		MaxItems:        aws.String("1"),
	}

	resp, err := r.client.ListResourceRecordSets(params)

	if _, ok := err.(awserr.Error); ok {
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			// a service error occurred
			log.Println(reqErr.Code(), reqErr.Message(), reqErr.StatusCode(), reqErr.RequestID())
		}
	}

	return resp.ResourceRecordSets, err
}

// updates DNS entry for the provided zoneId and record name
func (r *Route53Registry) UpdateDns(zoneId string, recordName string, action string, resourceRecords []*r53.ResourceRecord) error {

	params := &r53.ChangeResourceRecordSetsInput{
		ChangeBatch: &r53.ChangeBatch{ // Required
			Changes: []*r53.Change{ // Required
				&r53.Change{ // Required
					Action: aws.String(action), // Required
					ResourceRecordSet: &r53.ResourceRecordSet{ // Required
						Name:            aws.String(recordName), // Required
						Type:            aws.String("SRV"),      // Required
						ResourceRecords: resourceRecords,
						SetIdentifier:   aws.String("ResourceRecordSetIdentifier"),
						TTL:             aws.Long(1),
						Weight:          aws.Long(1),
					},
				},
			},
			Comment: aws.String(fmt.Sprintf("Adds a SRV DNS record for %s", recordName)),
		},
		HostedZoneID: aws.String(zoneId), // Required
	}
	_, err := r.client.ChangeResourceRecordSets(params)

	if _, ok := err.(awserr.Error); ok {
		// Generic AWS Error with Code, Message, and original error (if any)
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			// A service error occurred
			log.Println(fmt.Println(reqErr.Code(), reqErr.Message(), reqErr.StatusCode(), reqErr.RequestID()))
		}
	}

	return err
}

type ResourceRecords []*r53.ResourceRecord

// find the index of the record that contains the input string
func (slice ResourceRecords) pos(value string) int {
	for i, v := range slice {
		if strings.Contains(*v.Value, value) {
			return i
		}
	}
	return -1
}

type ResourceRecordSet []*r53.ResourceRecordSet

func (slice ResourceRecordSet) nameIs(name string) bool {
	if slice != nil && *slice[0].Name == name {
		return true
	} else {
		return false
	}
}

// Uses ec2 metadata service
// see http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html
func ec2Hostname() (string, error) {
	resp, err := http.Get("http://169.254.169.254/latest/meta-data/hostname")
	if err != nil {
		log.Fatal("Error getting hostname ", err)
	}

	defer resp.Body.Close()
	hostname, err := ioutil.ReadAll(resp.Body)

	return string(hostname[:]), err
}

func (r *Route53Registry) getHostname() string {
	// determine the hostname
	var hostname string
	if r.useEc2Meatadata {
		var hnerr error
		hostname, hnerr = ec2Hostname()
		if hnerr != nil {
			log.Fatal("Unable to determine EC2 hostname, defaulting to HOSTNAME")
			hostname, _ = os.Hostname()
		}
	} else {
		var hnerr error
		hostname, hnerr = os.Hostname()
		if hnerr != nil {
			log.Fatal("Can't get host name", hnerr)
		}
	}
	return hostname
}
