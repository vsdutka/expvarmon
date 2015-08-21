// expvar
package expvarmon

import (
	"log"
	"math"
	"sort"
	"sync"
)

type Var struct {
	sync.RWMutex
	name          string
	description   string
	unitname      string
	shortunitname string
}

type vars struct {
	mu sync.RWMutex
	vi map[string]struct {
		description   string
		unitname      string
		shortunitname string
	}
	keys []string
	vv   map[string]int64
}

// updateKeys updates the sorted list of keys in v.keys.
// must be called with v.mu held.
func (v *vars) updateKeys() {
	if len(v.vi) == len(v.keys) {
		// No new key.
		return
	}
	v.keys = v.keys[:0]
	for k := range v.vi {
		v.keys = append(v.keys, k)
	}
	sort.Strings(v.keys)
}

var v vars = vars{
	vi: make(map[string]struct {
		description   string
		unitname      string
		shortunitname string
	}),
	keys: make([]string, 0),
	vv:   make(map[string]int64),
}

func NewVar(name, description, unitname, shortunitname string) {
	v.mu.RLock()
	_, ok := v.vi[name]
	v.mu.RUnlock()
	if ok {
		log.Panicln("Reuse of exported var name:", name)
	}
	// check again under the write lock
	v.mu.Lock()
	v.vi[name] = struct {
		description   string
		unitname      string
		shortunitname string
	}{
		description,
		unitname,
		shortunitname,
	}
	v.vv[name] = 0
	v.updateKeys()
	v.mu.Unlock()
}

func Add(name string, delta int64) {
	v.mu.Lock()
	val, ok := v.vv[name]
	if !ok {
		log.Panicln("Unregistered var:", name)
	}
	v.vv[name] = val + delta
	v.mu.Unlock()
}

func Get(name string) int64 {
	v.mu.RLock()
	cur, ok := v.vv[name]
	if !ok {
		log.Panicln("Unregistered var:", name)
	}
	v.mu.RUnlock()
	return cur
}

func Set(name string, value int64) {
	v.mu.Lock()
	_, ok := v.vv[name]
	if !ok {
		log.Panicln("Unregistered var:", name)
	}
	v.vv[name] = value
	v.mu.Unlock()
}

func AddFloat(name string, delta float64) {
	v.mu.Lock()
	val, ok := v.vv[name]
	if !ok {
		log.Panicln("Unregistered var:", name)
	}
	curVal := math.Float64frombits(uint64(val))
	nxtVal := curVal + delta
	nxt := math.Float64bits(nxtVal)
	v.vv[name] = int64(nxt)
	v.mu.Unlock()
}

func GetFloat(name string) float64 {
	v.mu.RLock()
	cur, ok := v.vv[name]
	if !ok {
		log.Panicln("Unregistered var:", name)
	}
	v.mu.RUnlock()
	return math.Float64frombits(uint64(cur))
}

func SetFloat(name string, value float64) {
	v.mu.Lock()
	_, ok := v.vv[name]
	if !ok {
		log.Panicln("Unregistered var:", name)
	}
	v.vv[name] = int64(math.Float64bits(value))
	v.mu.Unlock()
}
