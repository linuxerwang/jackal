/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package router

import (
	"bytes"
	"encoding/gob"

	"github.com/ortuman/jackal/cluster"
	"github.com/ortuman/jackal/log"
)

type clusterDelegate struct {
	r *Router
}

func (d *clusterDelegate) NotifyMessage(msg []byte) {
	dec := gob.NewDecoder(bytes.NewReader(msg))
	cm := &clusterMessage{}
	cm.fromGob(dec)

	switch cm.typ {
	case messageBindType:
		break
	case messageUnbindType:
		break
	case messageSendType:
		break
	}
}

func (d *clusterDelegate) NodeJoined(node *cluster.Node) {
	log.Infof("join notified: %s", node.Name)
}

func (d *clusterDelegate) NodeUpdated(node *cluster.Node) {
	log.Infof("update notified: %s", node.Name)
}

func (d *clusterDelegate) NodeLeft(node *cluster.Node) {
	log.Infof("leave notified: %s", node.Name)
}
