package proxy

import (
	"fmt"
	"net/http"
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
			got := tt.a.Equals(tt.b)
			if got != tt.want {
				t.Errorf("want %t, got %t\n", tt.want, got)
			}
		})
	}
}

func TestPrefix(t *testing.T) {
	testCases := []struct {
		desc     string
		domain   string
		basepath string
		want     string
	}{
		{
			desc:   "match all domains 1",
			domain: "", basepath: "/",
			want: "/",
		},
		{
			desc:   "match all domains 2",
			domain: "*", basepath: "/",
			want: "/",
		},
		{
			desc:   "with domain add tld",
			domain: "domain", basepath: "/",
			want: "domain.wip/",
		},
	}
	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			bc := NewBackendConfig(tt.domain, tt.basepath, "ignore backend")
			got := bc.Prefix()
			if got != tt.want {
				t.Errorf("want %v, got %v\n", tt.want, got)
			}
		})
	}
}

func TestMatchHttpRequest(t *testing.T) {
	testCases := []struct {
		desc string
		url  string
		bc   *BackendConfig
		want bool
	}{
		{
			desc: "backend Config matches all domains, path should match",
			url:  "https://example.wip/starts/with/whatever",
			bc:   NewBackendConfig("", "/starts/with", "ignore backend"),
			want: true,
		},
		{
			desc: "domain matches, path matches",
			url:  "https://example.wip/starts/with/whatever",
			bc:   NewBackendConfig("example", "/starts/with", "ignore backend"),
			want: true,
		},
		{
			desc: "backend Config matches all domains, path does not match",
			url:  "https://example.wip/some/other/path",
			bc:   NewBackendConfig("", "/starts/with", "ignore backend"),
			want: false,
		},
		{
			desc: "domain matches, path does not match",
			url:  "https://example.wip/some/other/path",
			bc:   NewBackendConfig("example", "/starts/with", "ignore backend"),
			want: false,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.url, http.NoBody)
			if err != nil {
				t.Errorf("Error for url %v: %v\n", tt.url, err)
			}
			got := tt.bc.MatchHttpRequest(req)
			if got != tt.want {
				t.Errorf("want %v, got %v\nURL %s doesnt match %v\n", tt.want, got, tt.url, tt.bc)
			}
		})
	}
}
