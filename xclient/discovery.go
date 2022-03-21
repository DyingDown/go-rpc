package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
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
