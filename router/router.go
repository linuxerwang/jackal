/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package router

import (
	"crypto/tls"
	"errors"
	"sync"

	"github.com/ortuman/jackal/log"
	"github.com/ortuman/jackal/storage"
	"github.com/ortuman/jackal/stream"
	"github.com/ortuman/jackal/util"
	"github.com/ortuman/jackal/xmpp"
	"github.com/ortuman/jackal/xmpp/jid"
)

const defaultDomain = "localhost"

var (
	// ErrNotExistingAccount will be returned by Route method
	// if destination user does not exist.
	ErrNotExistingAccount = errors.New("router: account does not exist")

	// ErrResourceNotFound will be returned by Route method
	// if destination resource does not match any of user's available resources.
	ErrResourceNotFound = errors.New("router: resource not found")

	// ErrNotAuthenticated will be returned by Route method if
	// destination user is not available at this moment.
	ErrNotAuthenticated = errors.New("router: user not authenticated")

	// ErrBlockedJID will be returned by Route method if
	// destination JID matches any of the user's blocked JID.
	ErrBlockedJID = errors.New("router: destination jid is blocked")

	// ErrFailedRemoteConnect will be returned by Route method if
	// couldn't establish a connection to the remote server.
	ErrFailedRemoteConnect = errors.New("router: failed remote connection")
)

// S2SOutProvider provides a specific s2s outgoing connection for every single
// pair of (localdomai, remotedomain) values.
type S2SOutProvider interface {
	GetS2SOut(localDomain, remoteDomain string) (stream.S2SOut, error)
}

// Router represents an XMPP stanza router.
type Router struct {
	mu             sync.RWMutex
	s2sOutProvider S2SOutProvider
	hosts          map[string]tls.Certificate
	localStreams   map[string][]stream.C2S

	blockListsMu sync.RWMutex
	blockLists   map[string][]*jid.JID
}

// New returns an new empty router instance.
func New(config *Config) (*Router, error) {
	r := &Router{
		hosts:        make(map[string]tls.Certificate),
		blockLists:   make(map[string][]*jid.JID),
		localStreams: make(map[string][]stream.C2S),
	}
	if len(config.Hosts) > 0 {
		for _, h := range config.Hosts {
			r.hosts[h.Name] = h.Certificate
		}
	} else {
		cer, err := util.LoadCertificate("", "", defaultDomain)
		if err != nil {
			return nil, err
		}
		r.hosts[defaultDomain] = cer
	}
	return r, nil
}

// HostNames returns the list of all configured host names.
func (r *Router) HostNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var ret []string
	for n, _ := range r.hosts {
		ret = append(ret, n)
	}
	return ret
}

// IsLocalHost returns true if domain is a local server domain.
func (r *Router) IsLocalHost(domain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.hosts[domain]
	return ok
}

// Certificates returns an array of all configured domain certificates.
func (r *Router) Certificates() []tls.Certificate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var certs []tls.Certificate
	for _, cer := range r.hosts {
		certs = append(certs, cer)
	}
	return certs
}

// SetS2SOutProvider sets the s2s out provider to be used when routing stanzas remotely.
func (r *Router) SetS2SOutProvider(s2sOutProvider S2SOutProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s2sOutProvider = s2sOutProvider
}

// Bind marks a c2s stream as binded.
// An error will be returned in case no assigned resource is found.
func (r *Router) Bind(stm stream.C2S) {
	if len(stm.Resource()) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if authenticated := r.localStreams[stm.Username()]; authenticated != nil {
		r.localStreams[stm.Username()] = append(authenticated, stm)
	} else {
		r.localStreams[stm.Username()] = []stream.C2S{stm}
	}
	log.Infof("binded c2s stream... (%s/%s)", stm.Username(), stm.Resource())
	return
}

// Unbind unbinds a previously binded c2s.
// An error will be returned in case no assigned resource is found.
func (r *Router) Unbind(stm stream.C2S) {
	if len(stm.Resource()) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if resources := r.localStreams[stm.Username()]; resources != nil {
		res := stm.Resource()
		for i := 0; i < len(resources); i++ {
			if res == resources[i].Resource() {
				resources = append(resources[:i], resources[i+1:]...)
				break
			}
		}
		if len(resources) > 0 {
			r.localStreams[stm.Username()] = resources
		} else {
			delete(r.localStreams, stm.Username())
		}
	}
	log.Infof("unbinded c2s stream... (%s/%s)", stm.Username(), stm.Resource())
}

// UserStreams returns all streams associated to a user.
func (r *Router) UserStreams(username string) []stream.C2S {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.localStreams[username]
}

// IsBlockedJID returns whether or not the passed jid matches any
// of a user's blocking list JID.
func (r *Router) IsBlockedJID(jid *jid.JID, username string) bool {
	bl := r.getBlockList(username)
	for _, blkJID := range bl {
		if r.jidMatchesBlockedJID(jid, blkJID) {
			return true
		}
	}
	return false
}

// ReloadBlockList reloads in memory block list for a given user and starts
// applying it for future stanza routing.
func (r *Router) ReloadBlockList(username string) {
	r.blockListsMu.Lock()
	defer r.blockListsMu.Unlock()

	delete(r.blockLists, username)
	log.Infof("block list reloaded... (username: %s)", username)
}

// Route routes a stanza applying server rules for handling XML stanzas.
// (https://xmpp.org/rfcs/rfc3921.html#rules)
func (r *Router) Route(stanza xmpp.Stanza) error {
	return r.route(stanza, false)
}

// MustRoute routes a stanza applying server rules for handling XML stanzas
// ignoring blocking lists.
func (r *Router) MustRoute(stanza xmpp.Stanza) error {
	return r.route(stanza, true)
}

func (r *Router) jidMatchesBlockedJID(j, blockedJID *jid.JID) bool {
	if blockedJID.IsFullWithUser() {
		return j.Matches(blockedJID, jid.MatchesNode|jid.MatchesDomain|jid.MatchesResource)
	} else if blockedJID.IsFullWithServer() {
		return j.Matches(blockedJID, jid.MatchesDomain|jid.MatchesResource)
	} else if blockedJID.IsBare() {
		return j.Matches(blockedJID, jid.MatchesNode|jid.MatchesDomain)
	}
	return j.Matches(blockedJID, jid.MatchesDomain)
}

func (r *Router) getBlockList(username string) []*jid.JID {
	r.blockListsMu.RLock()
	bl := r.blockLists[username]
	r.blockListsMu.RUnlock()
	if bl != nil {
		return bl
	}
	blItms, err := storage.FetchBlockListItems(username)
	if err != nil {
		log.Error(err)
		return nil
	}
	bl = []*jid.JID{}
	for _, blItm := range blItms {
		j, _ := jid.NewWithString(blItm.JID, true)
		bl = append(bl, j)
	}
	r.blockListsMu.Lock()
	r.blockLists[username] = bl
	r.blockListsMu.Unlock()
	return bl
}

func (r *Router) route(element xmpp.Stanza, ignoreBlocking bool) error {
	toJID := element.ToJID()
	if !ignoreBlocking && !toJID.IsServer() {
		if r.IsBlockedJID(element.FromJID(), toJID.Node()) {
			return ErrBlockedJID
		}
	}
	if !r.IsLocalHost(toJID.Domain()) {
		return r.remoteRoute(element)
	}
	rcps := r.UserStreams(toJID.Node())
	if len(rcps) == 0 {
		exists, err := storage.UserExists(toJID.Node())
		if err != nil {
			return err
		}
		if exists {
			return ErrNotAuthenticated
		}
		return ErrNotExistingAccount
	}
	if toJID.IsFullWithUser() {
		for _, stm := range rcps {
			if stm.Resource() == toJID.Resource() {
				stm.SendElement(element)
				return nil
			}
		}
		return ErrResourceNotFound
	}
	switch element.(type) {
	case *xmpp.Message:
		// send to highest priority stream
		stm := rcps[0]
		var highestPriority int8
		if p := stm.Presence(); p != nil {
			highestPriority = p.Priority()
		}
		for i := 1; i < len(rcps); i++ {
			rcp := rcps[i]
			if p := rcp.Presence(); p != nil && p.Priority() > highestPriority {
				stm = rcp
				highestPriority = p.Priority()
			}
		}
		stm.SendElement(element)

	default:
		// broadcast toJID all streams
		for _, stm := range rcps {
			stm.SendElement(element)
		}
	}
	return nil
}

func (r *Router) remoteRoute(elem xmpp.Stanza) error {
	if r.s2sOutProvider == nil {
		return ErrFailedRemoteConnect
	}
	localDomain := elem.FromJID().Domain()
	remoteDomain := elem.ToJID().Domain()

	out, err := r.s2sOutProvider.GetS2SOut(localDomain, remoteDomain)
	if err != nil {
		log.Error(err)
		return ErrFailedRemoteConnect
	}
	out.SendElement(elem)
	return nil
}
