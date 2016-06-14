package main

import (
	_ "github.com/pipedrive/registrator/consul"
	_ "github.com/pipedrive/registrator/consulkv"
	_ "github.com/pipedrive/registrator/etcd"
	_ "github.com/pipedrive/registrator/skydns2"
	_ "github.com/pipedrive/registrator/zookeeper"
)
