package main

import (
	_ "github.com/RudolfRandal/registrator/consul"
	_ "github.com/RudolfRandal/registrator/consulkv"
	_ "github.com/RudolfRandal/registrator/etcd"
	_ "github.com/RudolfRandal/registrator/skydns2"
	_ "github.com/RudolfRandal/registrator/skydns2s"
	_ "github.com/RudolfRandal/registrator/zookeeper"
)
