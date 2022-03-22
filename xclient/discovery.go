package xclient

import (
	"errors"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
	ConsistentHashing
)

type Discovery interface {
	Refresh() error
	Update(servers []string) error
	Get(mode SelectMode) (string, error)
	GetAll() ([]string, error)
}

type MultiServerDiscovery struct {
	r       *rand.Rand
	lock    sync.Mutex
	servers []string
	index   int
}

type RegistryDiscovery struct {
	*MultiServerDiscovery
	registry   string
	timeout    time.Duration
	updatetime time.Time
}

const defaultUpdateTimeout = time.Second * 10

func NewRegistryDiscovery(registryAddr string, timeout time.Duration) *RegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	return &RegistryDiscovery{
		MultiServerDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:             registryAddr,
		timeout:              timeout,
	}
}

func NewMultiServerDiscovery(s []string) *MultiServerDiscovery {
	msd := &MultiServerDiscovery{
		servers: s,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	msd.index = msd.r.Intn(math.MaxInt32 - 1)
	return msd
}

var _ Discovery = (*MultiServerDiscovery)(nil)

func (msd *MultiServerDiscovery) Refresh() error {
	return nil
}

func (msd *MultiServerDiscovery) Update(servers []string) error {
	msd.lock.Lock()
	defer msd.lock.Unlock()
	msd.servers = servers
	return nil
}

func (msd *MultiServerDiscovery) Get(mode SelectMode) (string, error) {
	msd.lock.Lock()
	defer msd.lock.Unlock()
	length := len(msd.servers)
	if length == 0 {
		return "", errors.New("discovery: no available server")
	}
	if mode == RandomSelect {
		return msd.servers[msd.r.Intn(length)], nil
	} else if mode == RoundRobinSelect {
		s := msd.servers[msd.index%length]
		msd.index = (msd.index + 1) % length
		return s, nil
	} else if mode == ConsistentHashing {

	} else {
		return "", errors.New("not a supported select mode")
	}
}

func (msd *MultiServerDiscovery) GetAll() ([]string, error) {
	msd.lock.Lock()
	defer msd.lock.Unlock()
	servers := make([]string, len(msd.servers), len(msd.servers))
	copy(servers, msd.servers)
	return servers, nil
}

func (rd *RegistryDiscovery) Update(servers []string) error {
	rd.lock.Lock()
	defer rd.lock.Unlock()
	rd.servers = servers
	rd.updatetime = time.Now()
	return nil
}

func (rd *RegistryDiscovery) Refresh() error {
	rd.lock.Lock()
	defer rd.lock.Unlock()
	if rd.updatetime.Add(rd.timeout).After(time.Now()) {
		return nil
	}
	logrus.Info("registry: refresh servers from registry")
	respons, err := http.Get(rd.registry)
	if err != nil {
		return err
	}
	servers := strings.Split(respons.Header.Get("gorpc-servers"), ",")
	rd.servers = make([]string, 0, len(servers))
	for _, s := range servers {
		if strings.TrimSpace(s) != "" {
			rd.servers = append(rd.servers, strings.TrimSpace(s))
		}
	}
	rd.updatetime = time.Now()
	return nil
}

func (rd *RegistryDiscovery) Get(mode SelectMode) (string, error) {
	err := rd.Refresh()

	if err != nil {
		return "", err
	}
	return rd.MultiServerDiscovery.Get(mode)
}

func (rd *RegistryDiscovery) GetAll() ([]string, error) {
	err := rd.Refresh()
	if err != nil {
		return nil, err
	}
	return rd.MultiServerDiscovery.GetAll()
}
