package aws

import (
    "github.com/aws/aws-sdk-go/service/elbv2"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/aws"
    "fmt"
)

// LBInfo represents a ELBv2 endpoint
type LBInfo struct {
    DNSName string
    Port int64
}

// GetELBV2ForContainer returns an LBInfo struct with the load balancer DNS name and listener port for a given instanceId and port
// if an error occurs, or the target is not found, an empty LBInfo is returned.
func GetELBV2ForContainer(instanceID string, port int64) LBInfo {

    var lb []*string
    var lbPort *int64
    info := LBInfo{}

    sess, err := session.NewSession()
    if err != nil {
        fmt.Println("failed to create session,", err)
        return info 
    }
    svc := elbv2.New(sess)

    // Loop through target group pages and check for port and instanceID
    //
    params := &elbv2.DescribeTargetGroupsInput{
        PageSize: aws.Int64(10),

    }
    tgs, err := svc.DescribeTargetGroups(params)

    if err != nil {
        // Print the error, cast err to awserr.Error to get the Code and
        // Message from an error.
        fmt.Println(err.Error())
        return info
    }

    // Check each target group for a matching port and instanceID
    // We assume that there is only one LB for the target group (though the data structure allows more)
    for _, tg := range tgs.TargetGroups {
        params4 := &elbv2.DescribeTargetHealthInput{
            TargetGroupArn: aws.String(*tg.TargetGroupArn),
        }
        tarH, err := svc.DescribeTargetHealth(params4)

        for _, thd := range tarH.TargetHealthDescriptions {
            if *thd.Target.Port == port && *thd.Target.Id == instanceID {
                lb = tg.LoadBalancerArns
                lbPort = tg.Port
            }

        }
        if err != nil {
            fmt.Println(err.Error())
            return info
        }
        fmt.Printf("LB is: %v\n", *lb[0])
        fmt.Printf("LB Port is: %v\n", *lbPort)
    }

    params2 := &elbv2.DescribeLoadBalancersInput{
        LoadBalancerArns: lb,
    }
    lbData, err := svc.DescribeLoadBalancers(params2)

    if err != nil {
        fmt.Println(err.Error())
        return info
    }
    fmt.Printf("LB DNS: %s\n", *lbData.LoadBalancers[0].DNSName)

    info.DNSName = *lb[0]
    info.Port = *lbPort
    return info
}