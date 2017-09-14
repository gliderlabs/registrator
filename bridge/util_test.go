package bridge

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapedComma(t *testing.T) {
	cases := []struct {
		Tag      string
		Expected []string
	}{
		{
			Tag:      "",
			Expected: []string{},
		},
		{
			Tag:      "foobar",
			Expected: []string{"foobar"},
		},
		{
			Tag:      "foo,bar",
			Expected: []string{"foo", "bar"},
		},
		{
			Tag:      "foo\\,bar",
			Expected: []string{"foo,bar"},
		},
		{
			Tag:      "foo,bar\\,baz",
			Expected: []string{"foo", "bar,baz"},
		},
		{
			Tag:      "\\,foobar\\,",
			Expected: []string{",foobar,"},
		},
		{
			Tag:      ",,,,foo,,,bar,,,",
			Expected: []string{"foo", "bar"},
		},
		{
			Tag:      ",,,,",
			Expected: []string{},
		},
		{
			Tag:      ",,\\,,",
			Expected: []string{","},
		},
	}

	for _, c := range cases {
		results := recParseEscapedComma(c.Tag)
		sort.Strings(c.Expected)
		sort.Strings(results)
		assert.EqualValues(t, c.Expected, results)
	}
}

func TestEnvToMap(t *testing.T) {
	env := []string{
		"",
		"",
	}
	result := envToMap(env)
	assert.Equal(t, len(result), 0)

	env = []string{
		"key1=value1",
		"key2=value2",
	}
	result = envToMap(env)
	assert.Equal(t, len(result), 2)
	assert.Equal(t, result["key1"], "value1")
	assert.Equal(t, result["key2"], "value2")
}
