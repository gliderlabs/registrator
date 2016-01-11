package bridge

import (
	"strconv"
	"strings"

	"github.com/cenkalti/backoff"
	dockerapi "github.com/fsouza/go-dockerclient"
)

func retry(fn func() error) error {
	return backoff.Retry(fn, backoff.NewExponentialBackOff())
}

func mapDefault(m map[string]string, key, default_ string) string {
	v, ok := m[key]
	if !ok || v == "" {
		return default_
	}
	return v
}

func combineTags(tagParts ...string) []string {
	tags := make([]string, 0)
	for _, element := range tagParts {
		if element != "" {
			tags = append(tags, strings.Split(element, ",")...)
		}
	}
	return tags
}

func serviceMetaData(config *dockerapi.Config, port string, ipv6 bool) (map[string]string, map[string]bool) {
	meta := config.Env
	for k, v := range config.Labels {
		meta = append(meta, k+"="+v)
	}
	metadata           := make(map[string]string)
	metadataFromPort   := make(map[string]bool)
	metadataFromAF     := make(map[string]bool)
	metadataFromPortAF := make(map[string]bool)
	for _, kv := range meta {
		kvp := strings.SplitN(kv, "=", 2)
		if (strings.HasSuffix(kvp[0], "_IPV4") && ipv6) || (strings.HasSuffix(kvp[0], "_IPV6") && !ipv6) {
			// We can definitely ignore any key with the wrong address family suffix
			continue
		}
		if strings.HasPrefix(kvp[0], "SERVICE_") && len(kvp) > 1 {
			af := false
			if strings.HasSuffix(kvp[0], "_IPV4") {
				af = true
				kvp[0] = strings.TrimSuffix(kvp[0], "_IPV4")
			} else if strings.HasSuffix(kvp[0], "_IPV6") {
				af = true
				kvp[0] = strings.TrimSuffix(kvp[0], "_IPV6")
			}
			key := strings.ToLower(strings.TrimPrefix(kvp[0], "SERVICE_"))
			if metadataFromPortAF[key] || (!af && metadataFromPort[key]) {
				continue
			}
			portkey := strings.SplitN(key, "_", 2)
			_, err := strconv.Atoi(portkey[0])
			if err == nil && len(portkey) > 1 {
				if portkey[0] != port {
					continue
				}
				metadata[portkey[1]] = kvp[1]
				if af {
					metadataFromPortAF[portkey[1]] = true
				} else {
					metadataFromPort[portkey[1]] = true
				}
			} else {
				metadata[key] = kvp[1]
				if af {
					metadataFromAF[key] = true
				}
			}
		}
	}
	return metadata, metadataFromPort
}

func servicePort(container *dockerapi.Container, port dockerapi.Port, published []dockerapi.PortBinding) ServicePort {
	var hp, hip, ep, ept, eip, nm string
	if len(published) > 0 {
		hp = published[0].HostPort
		hip = published[0].HostIP
	}
	if hip == "" {
		hip = "0.0.0.0"
	}

	//for overlay networks
	//detect if container use overlay network, than set HostIP into NetworkSettings.Network[string].IPAddress
	//better to use registrator with -internal flag
	nm = container.HostConfig.NetworkMode
	if nm != "bridge" && nm != "default" && nm != "host" {
		hip = container.NetworkSettings.Networks[nm].IPAddress
	}

	exposedPort := strings.Split(string(port), "/")
	ep = exposedPort[0]
	if len(exposedPort) == 2 {
		ept = exposedPort[1]
	} else {
		ept = "tcp" // default
	}

	// Nir: support docker NetworkSettings
	eip = container.NetworkSettings.IPAddress
	if eip == "" {
		for _, network := range container.NetworkSettings.Networks {
			eip = network.IPAddress
		}
	}

	return ServicePort{
		HostPort:          hp,
		HostIP:            hip,
		ExposedPort:       ep,
		ExposedIP:         eip,
		PortType:          ept,
		ContainerID:       container.ID,
		ContainerHostname: container.Config.Hostname,
		container:         container,
	}
}
