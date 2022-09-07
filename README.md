
### Roadmap

* 方案一
  * 使用etcd raft demo的代码，将pebble db的持久化数据作为快照读出并发送给raft模块，但是这样实际使用的是raft.MemoryStorage，会带来持久化被重复保存两次的开销，而且并不能利用pebble本身持久化的功能
* 方案二
  * 重写etcd/storage接口，避免重复保存两次，工作量大且较为复杂
* 方案三
  * 重用memory storage中的data[]记录一个checkpoint的快照版本号，raft仅做快照版本号和endpoint的记录，load快照时通过RPC或者HTTP直接向对应endpoint请求快照发送之后读取快照
  * raft的快照仅保存pebble快照的元数据（addr和timestamp)，向别的节点同步快照时仅发送快照的元数据，通过元数据向对应的节点请求快照并更新自己的状态机
  
最终采用方案三实现

## docker问题

* m1编译报错 qemu-x86_64: Could not open '/lib/ld-musl-x86_64.so.1，由于编译的包是x86架构的，需要在from后面加参数 --platform=linux/amd64
* etcd不支持go 1.15，更换镜像为 fagongzi/golang:1.16.3
* alpine镜像自带的busybox的wget实现不支持 -m选项,所以需要再docker中重新安装wget

## pebble

pebble是一个go语言原生的KV存储数据库，利用vendor包管理直接在容器中源码编译

go get -d github.com/cockroachdb/pebble

### 快照和持久化

* pebble本身的snapshot仅仅记录一个当前的版本号，LSMTree中由于键值有序，由于多个记录的不同版本在内存中聚集，因此仅记录版本号创建一个当前时间的可读视图，不对持久化做任何保证
* checkpoint函数保证持久化的创建快照，需要使用绝对路径创建，可以创建一个db的只读副本或者用于打开一个独立的db
* 除了checkpoint之外不需要使用pebble.Sync将状态持久化到磁盘，commit的entry只需要写到memtable中就行
* 创建新快照后删除上一个快照，（因为pebbele db是在本次快照上运行的）。只能删除当前快照的上一个快照，但是有可能删除到已经发送元数据但是还未读取到快照，需要实现读取失败重试机制

## etcd

* 使用了etcd raftdemo中的内容
* 快照触发机制为，commitIndex-snapIndex>10000，触发后截断日志

## 测试

```sh
make docker
docker-compose up
cd pkg/client/
go test -run TestConsensus -timeout 0
```
