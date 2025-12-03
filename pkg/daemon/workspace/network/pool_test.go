package network_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/daemon/workspace/network"
	"github.com/stretchr/testify/suite"
)

type PoolTestSuite struct {
	suite.Suite
}

func (s *PoolTestSuite) TestNewPool() {
	pool := network.NewConnectionPool(5, 10)
	s.NotNil(pool)
}

func (s *PoolTestSuite) TestPoolReusesIdleConnection() {
	pool := network.NewConnectionPool(5, 10)

	// Create and return a connection
	conn1 := &testConn{id: "conn1"}
	err := pool.Put(conn1)
	s.NoError(err)

	// Get should return same connection
	conn2, err := pool.Get(context.Background())
	s.NoError(err)
	s.Equal(conn1, conn2)
}

func TestPoolTestSuite(t *testing.T) {
	suite.Run(t, new(PoolTestSuite))
}

// testConn for pool testing
type testConn struct {
	id string
}

func (t *testConn) Read(b []byte) (n int, err error)    { return 0, nil }
func (t *testConn) Write(b []byte) (n int, err error)   { return len(b), nil }
func (t *testConn) Close() error                        { return nil }
func (t *testConn) LocalAddr() net.Addr                 { return nil }
func (t *testConn) RemoteAddr() net.Addr                { return nil }
func (t *testConn) SetDeadline(t2 time.Time) error      { return nil }
func (t *testConn) SetReadDeadline(t2 time.Time) error  { return nil }
func (t *testConn) SetWriteDeadline(t2 time.Time) error { return nil }
