package store

import (
	"sync"

	"github.com/cockroachdb/pebble"
)

func newPebbleStore() (Store, error) {
	database, err := pebble.Open("demo", &pebble.Options{})
	if err != nil {
		panic("Fail to load pebble")
	}
	s := pebbleStore{db: database}
	return &s, nil
}

type pebbleStore struct {
	sync.RWMutex
	db *pebble.DB
}

func (p *pebbleStore) Set(key []byte, value []byte) error {
	p.Lock()
	defer p.Unlock()
	return p.db.Set(key, value, pebble.Sync)
}
func (p *pebbleStore) Get(key []byte) ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	value, _, err := p.db.Get(key)
	return value, err
}
func (p *pebbleStore) Delete(key []byte) error {
	p.Lock()
	p.Unlock()
	return p.db.Delete(key, pebble.Sync)
}
