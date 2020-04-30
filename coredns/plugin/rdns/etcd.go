package rdns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/rdns-server/coredns/plugin"
	"github.com/rancher/rdns-server/coredns/plugin/rdns/msg"

	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	etcdcv3 "go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/mvcc/mvccpb"
)

const (
	priority    = 10  // default priority when nothing is set
	ttl         = 300 // default ttl when nothing is set
	etcdTimeout = 5 * time.Second
)

var errKeyNotFound = errors.New("key not found")

type ETCD struct {
	Next          plugin.Handler
	Fall          fall.F
	Zones         []string
	PathPrefix    string
	Upstream      *upstream.Upstream
	Client        *etcdcv3.Client
	WildcardBound int8 // Calculate the boundary of WildcardDNS

	endpoints []string // Stored here as well, to aid in testing.
}

// Services implements the ServiceBackend interface.
func (e *ETCD) Services(ctx context.Context, state request.Request, exact bool, opt plugin.Options) ([]msg.Service, error) {
	services, err := e.Records(ctx, state, exact)
	if err != nil {
		return services, err
	}

	services = msg.Group(services)
	return services, err
}

// Reverse implements the ServiceBackend interface.
func (e *ETCD) Reverse(ctx context.Context, state request.Request, exact bool, opt plugin.Options) ([]msg.Service, error) {
	return e.Services(ctx, state, exact, opt)
}

// Lookup implements the ServiceBackend interface.
func (e *ETCD) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return e.Upstream.Lookup(ctx, state, name, typ)
}

// IsNameError implements the ServiceBackend interface.
func (e *ETCD) IsNameError(err error) bool {
	return err == errKeyNotFound
}

// Records looks up records in etcd. If exact is true, it will lookup just this
// name. This is used when find matches when completing SRV lookups for instance.
func (e *ETCD) Records(ctx context.Context, state request.Request, exact bool) ([]msg.Service, error) {
	name := state.Name()
	qType := state.QType()

	// No need to lookup the domain which is like zone name
	// for example:
	//  name: lb.rancher.cloud.
	//  zones: [lb.rancher.cloud]
	// "lb.rancher.cloud." shold not lookup any keys in etcd
	for _, zone := range e.Zones {
		if strings.HasPrefix(name, zone) {
			return nil, nil
		}
	}

	if e.WildcardBound > 0 && qType != dns.TypeTXT {
		temp := dns.SplitDomainName(name)
		if int8(len(temp)) > e.WildcardBound && !e.pathExist(ctx, temp) {
			start := int8(len(temp)) - e.WildcardBound
			name = fmt.Sprintf("*.%s", strings.Join(temp[start:], "."))
		}
	}

	path, star := msg.PathWithWildcard(name, e.PathPrefix)
	r, err := e.get(ctx, path, !exact)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(msg.Path(name, e.PathPrefix), "/")

	kvs := e.filterKvs(r.Kvs, segments, qType)

	return e.loopNodes(kvs, segments, star, state.QType())
}

func (e *ETCD) get(ctx context.Context, path string, recursive bool) (*etcdcv3.GetResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, etcdTimeout)
	defer cancel()
	if recursive == true {
		if !strings.HasSuffix(path, "/") {
			path = path + "/"
		}
		r, err := e.Client.Get(ctx, path, etcdcv3.WithPrefix())
		if err != nil {
			return nil, err
		}
		if r.Count == 0 {
			path = strings.TrimSuffix(path, "/")
			r, err = e.Client.Get(ctx, path)
			if err != nil {
				return nil, err
			}
			if r.Count == 0 {
				return nil, errKeyNotFound
			}
		}
		return r, nil
	}

	r, err := e.Client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if r.Count == 0 {
		return nil, errKeyNotFound
	}
	return r, nil
}

func (e *ETCD) loopNodes(kv []*mvccpb.KeyValue, nameParts []string, star bool, qType uint16) (sx []msg.Service, err error) {
	bx := make(map[msg.Service]struct{})
Nodes:
	for _, n := range kv {
		if star {
			s := string(n.Key)
			keyParts := strings.Split(s, "/")
			for i, n := range nameParts {
				if i > len(keyParts)-1 {
					// name is longer than key
					continue Nodes
				}
				if n == "*" || n == "any" {
					continue
				}
				if keyParts[i] != n {
					continue Nodes
				}
			}
		}
		serv := new(msg.Service)
		if err := json.Unmarshal(n.Value, serv); err != nil {
			return nil, fmt.Errorf("%s: %s", n.Key, err.Error())
		}
		serv.Key = string(n.Key)
		if _, ok := bx[*serv]; ok {
			continue
		}
		bx[*serv] = struct{}{}

		serv.TTL = e.TTL(n, serv)
		if serv.Priority == 0 {
			serv.Priority = priority
		}

		if shouldInclude(serv, qType) {
			sx = append(sx, *serv)
		}
	}
	return sx, nil
}

// TTL returns the smaller of the etcd TTL and the service's
// TTL. If neither of these are set (have a zero value), a default is used.
func (e *ETCD) TTL(kv *mvccpb.KeyValue, serv *msg.Service) uint32 {
	etcdTTL := uint32(kv.Lease)

	if etcdTTL == 0 && serv.TTL == 0 {
		return ttl
	}
	if etcdTTL == 0 {
		return serv.TTL
	}
	if serv.TTL == 0 {
		return etcdTTL
	}
	if etcdTTL < serv.TTL {
		return etcdTTL
	}
	return serv.TTL
}

// shouldInclude returns true if the service should be included in a list of records, given the qType. For all the
// currently supported lookup types, the only one to allow for an empty Host field in the service are TXT records.
// Similarly, the TXT record in turn requires the Text field to be set.
func shouldInclude(serv *msg.Service, qType uint16) bool {
	if qType == dns.TypeTXT {
		return serv.Text != ""
	}
	return serv.Host != ""
}

// filterKvs returns kvs which not contain sub domain records.
func (e *ETCD) filterKvs(kvs []*mvccpb.KeyValue, segments []string, qType uint16) []*mvccpb.KeyValue {
	if qType == dns.TypeA {
		result := make([]*mvccpb.KeyValue, 0)
		for _, v := range kvs {
			ss := strings.Split(string(v.Key), "/")
			s := segments[len(segments)-1:][0]
			p := `^\d{1,3}_\d{1,3}_\d{1,3}_\d{1,3}$`
			m, _ := regexp.MatchString(p, s)
			if s != "*" && m && e.WildcardBound == (int8(len(segments))-3) {
				continue
			}
			if s != "*" && len(ss)-len(segments) == 1 || s == "*" && len(ss)-(len(segments)-1) == 1 {
				result = append(result, v)
			}
		}
		return result
	}
	return kvs
}

func (e *ETCD) pathExist(ctx context.Context, ss []string) bool {
	ctx, cancel := context.WithTimeout(ctx, etcdTimeout)
	defer cancel()

	path, _ := msg.PathWithWildcard(strings.Join(ss, "."), e.PathPrefix)

	r, err := e.Client.Get(ctx, path, etcdcv3.WithPrefix())
	if err != nil {
		return false
	}

	if r.Count > 0 {
		return true
	}
	return false
}
