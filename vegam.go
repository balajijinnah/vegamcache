/*
Copyright 2018 The vegamcache Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vegamcache

import (
	"net"
	"time"

	"github.com/weaveworks/mesh"
)

type Vegam struct {
	gossip  mesh.Gossip
	peer    *peer
	router  *mesh.Router
	actions chan<- func()
	peers   []string
	stop    chan int
}

func NewVegam(vc *VegamConfig) (*Vegam, error) {
	initConfig(vc)
	peername, err := mesh.PeerNameFromString(vc.PeerName)
	if err != nil {
		return nil, err
	}
	router, err := mesh.NewRouter(
		mesh.Config{
			Port:               vc.Port,
			ProtocolMinVersion: mesh.ProtocolMinVersion,
			Password:           []byte(vc.Password),
			Host:               vc.Host,
			PeerDiscovery:      true,
			TrustedSubnets:     []*net.IPNet{},
		},
		peername,
		vc.NickName,
		mesh.NullOverlay{},
		vc.Logger,
	)
	if err != nil {
		return nil, err
	}
	peer := &peer{
		cache: &cache{
			set: make(map[string]Value),
		},
	}
	gossip, err := router.NewGossip(vc.Channel, peer)
	if err != nil {
		return nil, err
	}
	return &Vegam{
		gossip: gossip,
		peer:   peer,
		router: router,
		peers:  vc.Peers,
		stop:   make(chan int),
	}, nil
}

func (v *Vegam) Start() {
	actions := make(chan func())
	v.actions = actions
	v.router.Start()
	v.router.ConnectionMaker.InitiateConnections(v.peers, true)
	go v.loop(actions)
}

func (v *Vegam) loop(actions <-chan func()) {
	for {
		select {
		case f := <-actions:
			f()
		case <-v.stop:
			return
		}
	}
}

func (v *Vegam) Stop() {
	v.stop <- 1
	v.router.Stop()
}

func (v *Vegam) Get(key string) (val interface{}, exist bool) {
	val, exist = v.peer.cache.get(key)
	return
}

func (v *Vegam) Put(key string, val interface{}, expiry time.Duration) {
	var expiryTime int64
	if expiry == 0 {
		expiryTime = 0
	} else {
		expiryTime = time.Now().Add(expiry).UnixNano()
	}
	tempVal := Value{
		Data:      val,
		LastWrite: time.Now().UnixNano(),
		Expiry:    expiryTime,
	}
	v.peer.cache.put(key, tempVal)
	tempCache := &cache{
		set: make(map[string]Value),
	}
	tempCache.set[key] = tempVal
	v.actions <- func() {
		v.gossip.GossipBroadcast(tempCache)
	}
}
