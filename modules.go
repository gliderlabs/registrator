package main

import (
	_ "github.com/rudolfrandal/registrator/consul"
	_ "github.com/rudolfrandal/registrator/consulkv"
	_ "github.com/rudolfrandal/registrator/etcd"
	_ "github.com/rudolfrandal/registrator/skydns2"
	_ "github.com/rudolfrandal/registrator/skydns2s"
	_ "github.com/rudolfrandal/registrator/zookeeper"
)
