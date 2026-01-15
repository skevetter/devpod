package compose

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type HelperTestSuite struct {
	suite.Suite
}

func TestHelperSuite(t *testing.T) {
	suite.Run(t, new(HelperTestSuite))
}

func (s *HelperTestSuite) TestParseVersion() {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{
			name:    "standard semver",
			version: "2.37.1",
			want:    "2.37.1",
		},
		{
			name:    "semver with v prefix",
			version: "v2.37.1",
			want:    "2.37.1",
		},
		{
			name:    "ubuntu package version",
			version: "2.37.1+ds1-0ubuntu2~24.04.1",
			want:    "2.37.1",
		},
		{
			name:    "desktop version",
			version: "2.40.3-desktop.1",
			want:    "2.40.3",
		},
		{
			name:    "another ubuntu variant",
			version: "2.37.1+ds1-0ubuntu1~24",
			want:    "2.37.1",
		},
		{
			name:    "invalid version",
			version: "083f676",
			wantErr: true,
		},
		{
			name:    "empty version",
			version: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := parseVersion(tt.version)
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
				s.Equal(tt.want, got.String())
			}
		})
	}
}
