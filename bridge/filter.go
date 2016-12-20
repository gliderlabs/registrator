package bridge

import (
	"errors"
	"math"
	"net"
	"strconv"
	"strings"
)

type Filter struct {
	hostIP      bool        // if true, host ip is accepted (external)
	containerIP bool        // if true, container ip is accepted (internal)
	ip          net.IP      // ip address
	ipnet       *net.IPNet  // ip range
	portLess    bool        // if true, ip only (no port)
	portMin     uint16      // port range min
	portMax     uint16      // port range max
	proto       string      // protocol (tcp/udp)

}
type Filters struct {
	list []*Filter
}

// clear the filter lists
func (f *Filters) Clear() {
	f.list = []*Filter{}
}

// build a filter lists from input string
// input string is comma separated filter-string lists
// filter-string contains ip(or range):port(or range)/proto(tcp or udp) format
func (f *Filters) Append(input string) error {
	if len(input) == 0 {
		return nil
	}
	entries := strings.Split(input, ",")
	for _, entry := range entries {
		if len(entry) == 0 {
			return errors.New("empty filter entry exist")
		}
		filter := new(Filter)
		ipPort := strings.Split(entry, ":")
		if err := parseIp(ipPort[0], filter); err != nil {
			return err
		}
		if len(ipPort) > 1 {
			if err := parsePort(ipPort[1], filter); err != nil {
				return err
			}
		} else {
			filter.portLess = true
		}
		f.list = append(f.list, filter)
	}
	return nil
}

// check ip:port is matched with one of filter in list
// returned with matched filter for informations
func (f *Filters) Match(ip string, port uint16, internal bool) (bool, *Filter, error) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false, nil, errors.New("parse ip address error : " + ip)
	}
	for _, filter := range f.list {
		matched, _, err := matchIPPort(parsedIP, port, internal, filter)
		if err != nil {
			return false, nil, err
		}
		if matched {
			return true, filter, nil
		}
	}
	return false, nil, nil
}

// parse input string and generate filter ip parts
// input string must be ip (ex 192.168.1.1) or cidr (ex 192.168.0.0/16)
func parseIp(input string, filter *Filter) error {
	if strings.EqualFold(input, "host") {
		filter.hostIP = true
		return nil
	}
	if strings.EqualFold(input, "container") {
		filter.containerIP = true
		return nil
	}
	if strings.EqualFold(input, "0.0.0.0") {
		_, filter.ipnet, _ = net.ParseCIDR("0.0.0.0/0")
		return nil
	}
	if strings.Contains(input, "/") {
		_, ipnet, err := net.ParseCIDR(input)
		if err != nil {
			return err
		}
		filter.ipnet = ipnet
		return nil
	}
	ip := net.ParseIP(input)
	if ip == nil {
		return errors.New("parse ip address error : " + input)
	}
	filter.ip = ip
	return nil
}

// parse input string and generate filter port/port-range and protocol parts
// input string must be port (ex 80) or port range (ex 80-8080)
// and "/" separated protocol string can be appended (ex /udp)
func parsePort(input string, filter *Filter) error {
	portProto := strings.Split(input, "/")
	if len(portProto) >= 2 {
		if err := parseProto(portProto[1], filter); err != nil {
			return err
		}
	} else {
		filter.proto = "tcp"
	}
	// "*" means any port so min value is 0 and max value is uint16 max
	if strings.EqualFold(portProto[0], "*") {
		filter.portMin = 0
		filter.portMax = math.MaxUint16
		return nil
	}
	// if port string is separated with "-", it should be range of port
	// if port string isn't separated with "-", it should be a port
	ports := strings.Split(portProto[0], "-")
	v1, err := strconv.ParseUint(ports[0], 10, 16)
	if err != nil {
		return errors.New("parse port error : " + portProto[0])
	}
	filter.portMin = uint16(v1)
	if len(ports) >= 2 {
		v2, err := strconv.ParseUint(ports[1], 10, 16)
		if err != nil {
			return errors.New("parse port range error : " + portProto[0])
		}
		filter.portMax = uint16(v2)
	} else {
		filter.portMax = uint16(v1)
	}
	return nil
}

// parse protocol name (tcp or udp)
// if proto is not specified in input-string, doesn't call this function.
// so empty or strange strings must be returned error
func parseProto(input string, filter *Filter) error {
	if strings.EqualFold(input, "tcp") {
		filter.proto = "tcp"
		return nil
	}
	if strings.EqualFold(input, "udp") {
		filter.proto = "udp"
		return nil
	}
	return errors.New("parse protocol error : " + input)
}

// match ip:port with a filter
// at first check ip is matched with filter and if matched, check port is matched
func matchIPPort(ip net.IP, port uint16, internal bool, filter *Filter) (bool, *Filter, error) {
	// check ip
	// if filter is accept any host ip
	if filter.hostIP == true && internal == false {
		return matchPort(port, filter)
	}
	// if filter is accept any container ip
	if filter.containerIP == true && internal == true {
		return matchPort(port, filter)
	}
	// if filter is accept ip
	if filter.ip != nil {
		if filter.ip.Equal(ip) {
			return matchPort(port, filter)
		}
	}
	// if filter is accept ip range
	if filter.ipnet != nil {
		if filter.ipnet.Contains(ip) {
			return matchPort(port, filter)
		}
	}
	// not matched with filter ip/port
	return false, nil, nil
}

// match port with a filter
// portless means report ip address only (without port) so if portless is true, always return true
func matchPort(port uint16, filter *Filter) (bool, *Filter, error) {
	if filter.portLess == true {
		return true, filter, nil
	}
	if filter.portMin <= port && filter.portMax >= port {
		return true, filter, nil
	}
	return false, nil, nil
}