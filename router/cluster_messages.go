/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package router

import (
	"encoding/gob"

	"github.com/ortuman/jackal/xmpp"
	"github.com/ortuman/jackal/xmpp/jid"
)

type messageType int

const (
	messageBindType messageType = iota
	messageUnbindType
	messageSendType
)

type clusterMessage struct {
	typ  messageType
	node string
	jids []*jid.JID
	elem xmpp.XElement
}

func (cm *clusterMessage) fromGob(dec *gob.Decoder) {
	dec.Decode(&cm.typ)
	dec.Decode(&cm.node)

	switch cm.typ {
	case messageBindType, messageUnbindType:
		var l int
		dec.Decode(&l)
		for i := 0; i < l; i++ {
			var node, domain, resource string
			dec.Decode(&node)
			dec.Decode(&domain)
			dec.Decode(&resource)
			j, _ := jid.New(node, domain, resource, true)
			cm.jids = append(cm.jids, j)
		}

	case messageSendType:
		elem := &xmpp.Element{}
		elem.FromGob(dec)
		cm.elem = elem
	}
}

func (cm *clusterMessage) toGob(enc *gob.Encoder) {
	enc.Encode(cm.typ)
	enc.Encode(cm.node)
	switch cm.typ {
	case messageBindType, messageUnbindType:
		enc.Encode(len(cm.jids))
		for _, j := range cm.jids {
			enc.Encode(j.Node())
			enc.Encode(j.Domain())
			enc.Encode(j.Resource())
		}
	case messageSendType:
		cm.elem.ToGob(enc)
	}
}
