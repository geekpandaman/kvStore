package store

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"go.etcd.io/etcd/raft/v3/raftpb"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

var SnapShotDirs string = "/apps/matrixbase/distributed/snapshots"

type pebbleStore struct {
	sync.RWMutex //only lock for snapshot change
	db           *pebble.DB
	proposeC     chan<- string //channel for proposing updates
	snapshotter  *snap.Snapshotter
	nodeAddr     string
	ssinfo       *snapShotInfo
}

type snapShotInfo struct {
	Addr      string
	TimeStamp string //TODO: identify with addr and timestamp
}

func (s *snapShotInfo) toString() string {
	return s.Addr + s.TimeStamp
}

type kv struct {
	Key      []byte
	Val      []byte
	isDelete bool
}

func newPebbleStore(snapShotter *snap.Snapshotter, proposeC chan<- string, commitC <-chan *commit, errorC <-chan error, addr string) (Store, error) {
	p := pebbleStore{proposeC: proposeC, snapshotter: snapShotter, nodeAddr: addr, db: nil}
	snapShot, err := p.loadSnapshot()
	if err != nil {
		panic(err)
	}
	if snapShot == nil {
		log.Println("Init kv store in new folder")
		database, err := pebble.Open(filepath.Join(SnapShotDirs, strconv.FormatInt(time.Now().Unix(), 10)), &pebble.Options{})
		if err != nil {
			panic(err)
		}
		p.db = database
	} else {
		log.Printf("Init kv store in snapShot!")
		if err := p.recoverFromSnapshot(snapShot.Data); err != nil {
			log.Panic(err)
		}
	}
	go p.readCommits(commitC, errorC)
	return &p, nil
}

func (p *pebbleStore) Set(key []byte, value []byte) error {
	p.RLock()
	defer p.RUnlock()
	p.Propose(key, value, false)
	return nil
}
func (p *pebbleStore) Get(key []byte) ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	value, _, err := p.db.Get(key)
	return value, err
}
func (p *pebbleStore) Delete(key []byte) error {
	p.RLock()
	defer p.RUnlock()
	p.Propose(key, nil, true)
	return p.db.Delete(key, pebble.Sync)
}

func (p *pebbleStore) Propose(k []byte, v []byte, isDelete bool) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(kv{k, v, isDelete}); err != nil {
		log.Fatal(err)
	}
	p.proposeC <- buf.String()
}

func (p *pebbleStore) loadSnapshot() (*raftpb.Snapshot, error) {
	p.RLock()
	defer p.RUnlock()
	snapShot, err := p.snapshotter.Load()
	if err == snap.ErrNoSnapshot {
		return nil, nil
	}
	if err != nil {
		return snapShot, nil
	}
	log.Printf("%s load snapshot %s", p.nodeAddr, snapShot.Data)
	return snapShot, err
}

func (p *pebbleStore) readCommits(commitC <-chan *commit, errorC <-chan error) {
	for commit := range commitC {
		if commit == nil {
			// signaled to load snapshot
			snapshot, err := p.loadSnapshot()
			if err != nil {
				log.Panic(err)
			}
			if snapshot != nil {
				log.Printf("loading snapshot at term %d and index %d", snapshot.Metadata.Term, snapshot.Metadata.Index)
				if err := p.recoverFromSnapshot(snapshot.Data); err != nil {
					log.Panic(err)
				}
			}
			continue
		}

		for _, data := range commit.data {
			var dataKv kv
			dec := gob.NewDecoder(bytes.NewBufferString(data))
			if err := dec.Decode(&dataKv); err != nil {
				log.Fatalf("raftexample: could not decode message (%v)", err)
			}
			if dataKv.isDelete {
				p.db.Delete(dataKv.Key, pebble.NoSync)
			} else {
				p.db.Set(dataKv.Key, dataKv.Val, pebble.NoSync)
			}

		}
		close(commit.applyDoneC)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (p *pebbleStore) GetSnapshot() ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	p.db.Flush()
	err := p.db.Checkpoint(filepath.Join(SnapShotDirs, timestamp))
	if err != nil {
		log.Println(err)
		return nil, err
	} else {
		log.Println(fmt.Sprintf("Success create checkpoint %s", timestamp))
	}
	// p.Lock()
	// defer p.Unlock()
	ssinfo := snapShotInfo{Addr: p.nodeAddr, TimeStamp: timestamp}
	log.Println(fmt.Sprintf("Success create ssinfo %s", ssinfo))
	p.ssinfo = &ssinfo
	return json.Marshal(ssinfo)
}

//if addr did not equals this node, wget from another server
func (p *pebbleStore) recoverFromSnapshot(snapshot []byte) error {
	log.Println("recover from snapshot!")
	// p.Lock()
	// defer p.Unlock()
	ssinfo := &snapShotInfo{}
	if err := json.Unmarshal(snapshot, ssinfo); err != nil {
		log.Println("fail to unmarshall")
		return err
	}
	if ssinfo.Addr != p.nodeAddr {
		cmd := exec.Command("wget", "-r", "-np", "-nH", filepath.Join(ssinfo.Addr, "snapshots", ssinfo.TimeStamp)+"/")
		log.Println(cmd)
		stdout, err := cmd.Output()
		fmt.Println(stdout)
		if err != nil {
			log.Println(err)
			return err
		}
	}
	dir := filepath.Join(SnapShotDirs, ssinfo.TimeStamp)
	if _, err := os.Stat(dir); err != nil {
		log.Println(err)
		return nil
	}
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		log.Println(err)
		return err
	}
	p.Lock()
	defer p.Unlock()
	//reopen p.db
	if p.db != nil {
		if err := p.db.Close(); err != nil {
			log.Println(err)
			return err
		}
	}
	p.db = db
	p.ssinfo = ssinfo
	log.Printf("%s successfully recover to snapShot %s from %s", p.nodeAddr, ssinfo.TimeStamp, ssinfo.Addr)
	return nil
}
