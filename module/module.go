/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package module

import (
	"context"

	"github.com/ortuman/jackal/module/offline"
	"github.com/ortuman/jackal/module/roster"
	"github.com/ortuman/jackal/module/xep0012"
	"github.com/ortuman/jackal/module/xep0030"
	"github.com/ortuman/jackal/module/xep0049"
	"github.com/ortuman/jackal/module/xep0054"
	"github.com/ortuman/jackal/module/xep0077"
	"github.com/ortuman/jackal/module/xep0092"
	"github.com/ortuman/jackal/module/xep0191"
	"github.com/ortuman/jackal/module/xep0199"
	"github.com/ortuman/jackal/router"
	"github.com/ortuman/jackal/stream"
	"github.com/ortuman/jackal/xmpp"
)

// Module represents a generic XMPP module.
type Module interface {
}

// IQHandler represents an IQ handler module.
type IQHandler interface {
	Module

	// MatchesIQ returns whether or not an IQ should be
	// processed by the module.
	MatchesIQ(iq *xmpp.IQ) bool

	// ProcessIQ processes a module IQ taking according actions
	// over the associated stream.
	ProcessIQ(iq *xmpp.IQ, stm stream.C2S)
}

// Modules structure keeps reference to a set of preconfigured modules.
type Modules struct {
	Roster       *roster.Roster
	Offline      *offline.Offline
	LastActivity *xep0012.LastActivity
	Private      *xep0049.Private
	DiscoInfo    *xep0030.DiscoInfo
	VCard        *xep0054.VCard
	Register     *xep0077.Register
	Version      *xep0092.Version
	BlockingCmd  *xep0191.BlockingCommand
	Ping         *xep0199.Ping

	iqHandlers  []IQHandler
	all         []Module
	shutdownChs []chan<- chan bool
}

// New returns a set of modules derived from a concrete configuration.
func New(config *Config, router *router.Router) *Modules {
	var shutdownCh chan<- chan bool
	m := &Modules{}

	// XEP-0030: Service Discovery (https://xmpp.org/extensions/xep-0030.html)
	m.DiscoInfo, shutdownCh = xep0030.New(router)
	m.iqHandlers = append(m.iqHandlers, m.DiscoInfo)
	m.all = append(m.all, m.DiscoInfo)
	m.shutdownChs = append(m.shutdownChs, shutdownCh)

	// Roster (https://xmpp.org/rfcs/rfc3921.html#roster)
	if _, ok := config.Enabled["roster"]; ok {
		m.Roster, shutdownCh = roster.New(&config.Roster, router)
		m.iqHandlers = append(m.iqHandlers, m.Roster)
		m.all = append(m.all, m.Roster)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0012: Last Activity (https://xmpp.org/extensions/xep-0012.html)
	if _, ok := config.Enabled["last_activity"]; ok {
		m.LastActivity, shutdownCh = xep0012.New(m.DiscoInfo, router)
		m.iqHandlers = append(m.iqHandlers, m.LastActivity)
		m.all = append(m.all, m.LastActivity)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0049: Private XML Storage (https://xmpp.org/extensions/xep-0049.html)
	if _, ok := config.Enabled["private"]; ok {
		m.Private, shutdownCh = xep0049.New()
		m.iqHandlers = append(m.iqHandlers, m.Private)
		m.all = append(m.all, m.Private)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0054: vcard-temp (https://xmpp.org/extensions/xep-0054.html)
	if _, ok := config.Enabled["vcard"]; ok {
		m.VCard, shutdownCh = xep0054.New(m.DiscoInfo)
		m.iqHandlers = append(m.iqHandlers, m.VCard)
		m.all = append(m.all, m.VCard)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0077: In-band registration (https://xmpp.org/extensions/xep-0077.html)
	if _, ok := config.Enabled["registration"]; ok {
		m.Register, shutdownCh = xep0077.New(&config.Registration, m.DiscoInfo)
		m.iqHandlers = append(m.iqHandlers, m.Register)
		m.all = append(m.all, m.Register)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0092: Software Version (https://xmpp.org/extensions/xep-0092.html)
	if _, ok := config.Enabled["version"]; ok {
		m.Version, shutdownCh = xep0092.New(&config.Version, m.DiscoInfo)
		m.iqHandlers = append(m.iqHandlers, m.Version)
		m.all = append(m.all, m.Version)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0160: Offline message storage (https://xmpp.org/extensions/xep-0160.html)
	if _, ok := config.Enabled["offline"]; ok {
		m.Offline, shutdownCh = offline.New(&config.Offline, m.DiscoInfo, router)
		m.all = append(m.all, m.Offline)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0191: Blocking Command (https://xmpp.org/extensions/xep-0191.html)
	if _, ok := config.Enabled["blocking_command"]; ok {
		m.BlockingCmd, shutdownCh = xep0191.New(m.DiscoInfo, m.Roster, router)
		m.iqHandlers = append(m.iqHandlers, m.BlockingCmd)
		m.all = append(m.all, m.BlockingCmd)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}

	// XEP-0199: XMPP Ping (https://xmpp.org/extensions/xep-0199.html)
	if _, ok := config.Enabled["ping"]; ok {
		m.Ping, shutdownCh = xep0199.New(&config.Ping, m.DiscoInfo)
		m.iqHandlers = append(m.iqHandlers, m.Ping)
		m.all = append(m.all, m.Ping)
		m.shutdownChs = append(m.shutdownChs, shutdownCh)
	}
	return m
}

// ProcessIQ process a module IQ returning 'service unavailable'
// in case it can't be properly handled.
func (m *Modules) ProcessIQ(iq *xmpp.IQ, stm stream.C2S) {
	for _, handler := range m.iqHandlers {
		if !handler.MatchesIQ(iq) {
			continue
		}
		handler.ProcessIQ(iq, stm)
		return
	}

	// ...IQ not handled...
	if iq.IsGet() || iq.IsSet() {
		stm.SendElement(iq.ServiceUnavailableError())
	}
}

func (m *Modules) Shutdown(ctx context.Context) error {
	select {
	case <-m.shutdown():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Modules) shutdown() <-chan bool {
	c := make(chan bool)
	go func() {
		// shutdown modules in reverse order
		for i := len(m.shutdownChs) - 1; i >= 0; i-- {
			shutdownCh := m.shutdownChs[i]
			wc := make(chan bool, 1)
			shutdownCh <- wc
			<-wc
		}
		close(c)
	}()
	return c
}
