package client

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSet(t *testing.T) {
	c := New("127.0.0.1:8081")

	key := []byte("key")
	value := []byte("value")
	err := c.Set(key, value)
	assert.NoError(t, err, "TestSet failed")
}

func TestGet(t *testing.T) {
	c := New("127.0.0.1:8081")

	key := []byte("key")
	value := []byte("value")
	err := c.Set(key, value)
	assert.NoError(t, err, "TestGet failed")

	v, err := c.Get(key)
	assert.NoError(t, err, "TestGet failed")
	assert.Equal(t, string(value), string(v), "TestGet failed")
}

func TestDelete(t *testing.T) {
	c := New("127.0.0.1:8081")

	key := []byte("key")
	value := []byte("value")
	err := c.Set(key, value)
	assert.NoError(t, err, "TestDelete failed")

	v, err := c.Get(key)
	assert.NoError(t, err, "TestDelete failed")
	assert.Equal(t, string(value), string(v), "TestDelete failed")

	assert.NoError(t, c.Delete(key), "TestDelete failed")

	v, err = c.Get(key)
	assert.NoError(t, err, "TestDelete failed")
	assert.Empty(t, v, "TestDelete failed")
}

func TestConsensus(t *testing.T) {
	var mu sync.Mutex
	data := make(map[string][]byte)
	var failTime int32
	//write to two nodes
	cmd := exec.Command("docker", "stop", "consensus_node3_1")
	err := cmd.Run()
	assert.NoError(t, err, "Fail to stop a node!")

	ticker := time.NewTicker(time.Second)
	stopCh := make(chan bool)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case t := <-ticker.C:
				go func(timestamp string) {
					fmt.Printf("Start set at %s\n", timestamp)
					c := New("127.0.0.1:8081")
					for i := 0; i != 1000; i++ {
						key := []byte(timestamp + fmt.Sprint(i))
						value := key
						err := c.Set(key, value)
						if err != nil {
							atomic.AddInt32(&failTime, 1)
						} else {
							mu.Lock()
							data[string(key)] = value
							mu.Unlock()
						}
					}
					fmt.Printf("Stop set at %s\n", timestamp)
				}(strconv.FormatInt(t.Unix(), 10))

			case <-stopCh:
				return
			}
		}
	}()

	time.Sleep(5 * time.Minute)
	stopCh <- true
	//restart the node
	cmd = exec.Command("docker", "restart", "consensus_node3_1")
	err = cmd.Run()
	assert.NoError(t, err, "Fail to stop a node!")
	time.Sleep(time.Minute)
	fmt.Println("Start test data consensus")
	c := New("127.0.0.1:8083")
	for k, v := range data {
		val, err := c.Get([]byte(k))
		assert.NoError(t, err)
		assert.Equal(t, v, val)
	}
}
