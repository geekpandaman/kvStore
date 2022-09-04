package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/matrixorigin/talent-challenge/matrixbase/distributed/pkg/cfg"
	"github.com/matrixorigin/talent-challenge/matrixbase/distributed/pkg/model"
	"github.com/matrixorigin/talent-challenge/matrixbase/distributed/pkg/store"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

// Server http api server
type Server struct {
	cfg        cfg.Cfg
	store      store.Store
	engine     *gin.Engine
	proposeC   chan string
	confchange chan raftpb.ConfChange
}

// NewServer create the server
func NewServer(cfg cfg.Cfg) (*Server, error) {
	proposeC := make(chan string)
	confChangeC := make(chan raftpb.ConfChange)
	var kvs store.Store
	getSnapshot := func() ([]byte, error) { return kvs.GetSnapshot() }
	commitC, errorC, snapshotterReady := store.NewRaftNode(cfg.Raft.ID, strings.Split(cfg.Raft.Peers, ","), false, getSnapshot, proposeC, confChangeC)

	kvs, err := store.NewStore(cfg.Store, <-snapshotterReady, proposeC, commitC, errorC, cfg.API.Addr)
	if err != nil {
		return nil, err
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	return &Server{
		cfg:    cfg,
		store:  kvs,
		engine: engine,
	}, nil
}

// Start start the server
func (s *Server) Start() error {
	s.engine.GET("/kv", s.doGet)
	s.engine.POST("/kv", s.doSet)
	s.engine.DELETE("/kv", s.doDelete)
	//serve snapshot directory
	s.engine.StaticFS("/snapshots", http.Dir(store.SnapShotDirs))
	return s.engine.Run(s.cfg.API.Addr)
}

// Stop stop the server
func (s *Server) Stop() error {
	close(s.confchange)
	close(s.proposeC)
	return nil
}

func (s *Server) doGet(c *gin.Context) {
	key := []byte(c.Query("key"))
	value, err := s.store.Get(key)
	if err != nil {
		c.JSON(http.StatusOK, returnError(err))
		return
	}

	c.JSON(http.StatusOK, returnData(value))
}

func (s *Server) doSet(c *gin.Context) {
	req := &model.Request{}
	err := c.ShouldBindJSON(req)
	if err != nil {
		c.JSON(http.StatusOK, returnError(err))
		return
	}

	err = s.store.Set([]byte(req.Key), []byte(req.Value))
	if err != nil {
		c.JSON(http.StatusOK, returnError(err))
		return
	}

	c.JSON(http.StatusOK, returnData("OK"))
}

func (s *Server) doDelete(c *gin.Context) {
	key := []byte(c.Query("key"))
	err := s.store.Delete(key)
	if err != nil {
		c.JSON(http.StatusOK, returnError(err))
		return
	}

	c.JSON(http.StatusOK, returnData("OK"))
}
