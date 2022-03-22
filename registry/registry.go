package registry

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Registry struct {
	timeout time.Duration
	lock    sync.Mutex
	servers map[string]*ServerItem
}

type ServerItem struct {
	addr      string
	startTime time.Time
}

const (
	DefaultPath    = "/gorpc/registry"
	defaultTimeout = time.Minute * 5
)

func NewRegistry(timeout time.Duration) *Registry {
	return &Registry{
		timeout: timeout,
		servers: make(map[string]*ServerItem),
	}
}

var DefaultRegistry = NewRegistry(defaultTimeout)

func (r *Registry) putServer(addr string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	s, ok := r.servers[addr]
	if ok {
		s.startTime = time.Now()
	} else {
		r.servers[addr] = &ServerItem{
			addr:      addr,
			startTime: time.Now(),
		}
	}
}

func (r *Registry) aliveServers() []string {
	r.lock.Lock()
	defer r.lock.Unlock()
	var alive []string
	for addr, server := range r.servers {
		if r.timeout == 0 || server.startTime.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Method == "GET" {
		w.Header().Set("gorpc-servers", strings.Join(r.aliveServers(), ","))
	} else if request.Method == "POST" {
		addr := request.Header.Get("gorpc-server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *Registry) HanddleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	logrus.Infof("rpc path: %s", registryPath)
}

func HandleHTTP() {
	DefaultRegistry.HanddleHTTP(DefaultPath)
}

func Hearbeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		duration = defaultTimeout - time.Minute
	}
	var err error

	err = sendHearbeat(registry, addr)
	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			err = sendHearbeat(registry, addr)
		}
	}()
}

func sendHearbeat(registry, addr string) error {
	logrus.Infof("server %s sent heartbeat to registry %s", addr, registry)
	httpClient := &http.Client{
		// Timeout: time.Duration(1),
	}
	request, _ := http.NewRequest("POST", registry, nil)
	request.Header.Set("gorpc-server", addr)
	_, err := httpClient.Do(request)
	if err != nil {
		logrus.Errorf("server: send hearbeat error: %v", err)
		return err
	}

	return nil
}
