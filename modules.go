package main

import (
	_ "github.com/vadzappa/registrator/consul"
	_ "github.com/vadzappa/registrator/consulkv"
	_ "github.com/vadzappa/registrator/etcd"
	_ "github.com/vadzappa/registrator/skydns2"
	_ "github.com/vadzappa/registrator/zookeeper"
)
