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
			NewBackendConfig("a", "b", "c"),
			NewBackendConfig("a", "b", "c"),
		},
		{
			"Domain not equal", false,
			NewBackendConfig("a", "b", "c"),
			NewBackendConfig("x", "b", "c"),
		},
		{
			"Basepath not equal", false,
			NewBackendConfig("a", "b", "c"),
			NewBackendConfig("a", "", "c"),
		},
		{
			"BackendBaseUrl not equal", false,
			NewBackendConfig("a", "b", "c"),
			NewBackendConfig("a", "b", "cc"),
		},
		{
			"nil", false,
			NewBackendConfig("a", "b", "c"),
			nil,
		},
		{
			"Domain '*' should be the same as empty string", true,
			NewBackendConfig("", "b", "c"),
			NewBackendConfig("*", "b", "c"),
		},
		{
			"Domain '*' should be the same as empty string 2", true,
			NewBackendConfig("*", "b", "c"),
			NewBackendConfig("", "b", "c"),
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
