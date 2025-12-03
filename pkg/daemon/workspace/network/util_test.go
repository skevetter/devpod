package network

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type UtilTestSuite struct {
	suite.Suite
}

func TestUtilTestSuite(t *testing.T) {
	suite.Run(t, new(UtilTestSuite))
}

func (s *UtilTestSuite) TestParseHostPort() {
	host, port, err := ParseHostPort("localhost:8080")
	s.NoError(err)
	s.Equal("localhost", host)
	s.Equal(8080, port)
}

func (s *UtilTestSuite) TestParseHostPortIPv6() {
	host, port, err := ParseHostPort("[::1]:8080")
	s.NoError(err)
	s.Equal("::1", host)
	s.Equal(8080, port)
}

func (s *UtilTestSuite) TestParseHostPortInvalid() {
	_, _, err := ParseHostPort("invalid")
	s.Error(err)
}

func (s *UtilTestSuite) TestFormatHostPort() {
	addr := FormatHostPort("localhost", 8080)
	s.Equal("localhost:8080", addr)
}

func (s *UtilTestSuite) TestIsLocalhost() {
	s.True(IsLocalhost("localhost"))
	s.True(IsLocalhost("127.0.0.1"))
	s.True(IsLocalhost("::1"))
	s.False(IsLocalhost("192.168.1.1"))
}

func (s *UtilTestSuite) TestGetFreePort() {
	port, err := GetFreePort()
	s.NoError(err)
	s.Greater(port, 0)
	s.Less(port, 65536)
}
