package giturls

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
    inputs := []string{
        "http://github.com/clbiggs/git-sync.git",
        "git@github.com:clbiggs/git-sync.git",
        "/home/user/repo",
    }

    outputs := [][]string {
        {"http","","github.com"},
        {"ssh","git","github.com"},
        {"file", "", ""},
    }

    for i, input := range inputs {
        url, err := Parse(input)
        assert.Nil(t, err)
        assert.Equal(t, url.Scheme, outputs[i][0])
        assert.Equal(t, url.User.Username(), outputs[i][1])
        assert.Equal(t, url.Host, outputs[i][2])
    }
}
