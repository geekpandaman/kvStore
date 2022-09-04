package store

import (
	"github.com/matrixorigin/talent-challenge/matrixbase/distributed/pkg/cfg"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

// Store the store interface
type Store interface {
	// Set set key-value to store
	Set(key []byte, value []byte) error
	// Get returns the value from store
	Get(key []byte) ([]byte, error)
	// Delete remove the key from store
	Delete(key []byte) error
	// Get snapshot from store
	GetSnapshot() ([]byte, error)
}

// NewStore create the raft store
func NewStore(cfg cfg.StoreCfg, snapshotter *snap.Snapshotter, proposeC chan<- string, commitC <-chan *commit, errorC <-chan error, addr string) (Store, error) {
	if cfg.Memory {
		panic("Memory store is not support")
	}

	return newPebbleStore(snapshotter, proposeC, commitC, errorC, addr)
}
