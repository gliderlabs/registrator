package main

import (
	"net"
	"net/url"
	"strconv"
	"time"
	"errors"
	"strings"

	"github.com/samuel/go-zookeeper/zk"
)

type ZookeeperRegistry struct {
	client *zk.Conn
	path   string
}

func NewZookeeperRegistry(uri *url.URL) ServiceRegistry {
  c, _, _ := zk.Connect([]string{uri.Host}, time.Second)
	return &ZookeeperRegistry{client: c, path: uri.Path}
}

func (r *ZookeeperRegistry) Register(service *Service) error {
	pathBase := r.path + "/" + service.Name
	path := pathBase + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
  err := ZookeeperEnsurePath(r.client, pathBase)
  if err != nil {
    return err
  }
	_, err = r.client.Create(path, []byte(addr), 0, zk.WorldACL(zk.PermAll))
	return err
}

func (r *ZookeeperRegistry) Deregister(service *Service) error {
	path := r.path + "/" + service.Name + "/" + service.ID
	err := r.client.Delete(path, -1)
	return err
}

func (r *ZookeeperRegistry) Refresh(service *Service) error {
	return errors.New("Zookeeper backend does not support refresh (TODO: Ephemeral?? or https://issues.apache.org/jira/browse/ZOOKEEPER-1925i needed)")
}

func ZookeeperEnsurePath(client *zk.Conn, path string) error {
  currPath := ""
  for _, currElem := range strings.Split(path,"/"){
    if currElem == ""{
        continue
    }
    currPath = currPath + "/" + currElem
    exist, _, err := client.Exists(currPath)
    if err != nil {
      return err
    }
    if exist == false{
      _, err := client.Create(currPath, nil, 0, zk.WorldACL(zk.PermAll))
      return err
    }
  }
  return nil
}
