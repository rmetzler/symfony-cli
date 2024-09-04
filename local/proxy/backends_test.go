package proxy

import (
	"fmt"
	"testing"
)

func TestEquals(t *testing.T) {
	testCases := []struct {
		desc string
		want bool
		a, b *BackendConfig
	}{
		{
			"equals", true,
			&BackendConfig{Domain: "a", Basepath: "b", BackendBaseUrl: "c"},
			&BackendConfig{Domain: "a", Basepath: "b", BackendBaseUrl: "c"},
		},
		{
			"Domain not equal", false,
			&BackendConfig{Domain: "a", Basepath: "b", BackendBaseUrl: "c"},
			&BackendConfig{Domain: "x", Basepath: "b", BackendBaseUrl: "c"},
		},
		{
			"Basepath not equal", false,
			&BackendConfig{Domain: "a", Basepath: "b", BackendBaseUrl: "c"},
			&BackendConfig{Domain: "a", Basepath: "", BackendBaseUrl: "c"},
		},
		{
			"BackendBaseUrl not equal", false,
			&BackendConfig{Domain: "a", Basepath: "b", BackendBaseUrl: "c"},
			&BackendConfig{Domain: "a", Basepath: "b", BackendBaseUrl: "cc"},
		},
		{
			"nil", false,
			&BackendConfig{Domain: "a", Basepath: "b", BackendBaseUrl: "c"},
			nil,
		},
		{
			"Domain '*' should be the same as empty string", true,
			&BackendConfig{Domain: "", Basepath: "b", BackendBaseUrl: "c"},
			&BackendConfig{Domain: "*", Basepath: "b", BackendBaseUrl: "c"},
		},
		{
			"Domain '*' should be the same as empty string 2", true,
			&BackendConfig{Domain: "*", Basepath: "b", BackendBaseUrl: "c"},
			&BackendConfig{Domain: "", Basepath: "b", BackendBaseUrl: "c"},
		},
	}
	for _, tt := range testCases {
		testname := fmt.Sprintf(
			`TEST "%s" equals expected to be %v [%#v == %#v]`,
			tt.desc, tt.want, tt.a, tt.b,
		)
		t.Run(testname, func(t *testing.T) {
			ans := tt.a.Equals(tt.b)
			if ans != tt.want {
				t.Errorf("want %t, got %t", tt.want, ans)
			}
		})
	}
}
