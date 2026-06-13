package main

import (
	"sort"
	"strings"
	"testing"
)

func TestDiffCollaborators(t *testing.T) {
	owner := "owner@example.com"

	cases := []struct {
		name       string
		desired    []string
		current    []string // current member emails (excluding owner)
		wantAdd    []string
		wantRemove []string
	}{
		{
			name:       "add and remove",
			desired:    []string{"a@x.com", "b@x.com"},
			current:    []string{"b@x.com", "c@x.com"},
			wantAdd:    []string{"a@x.com"},
			wantRemove: []string{"c@x.com"},
		},
		{
			name:       "all in sync",
			desired:    []string{"a@x.com", "b@x.com"},
			current:    []string{"a@x.com", "b@x.com"},
			wantAdd:    nil,
			wantRemove: nil,
		},
		{
			name:       "empty current means add all",
			desired:    []string{"a@x.com", "b@x.com"},
			current:    nil,
			wantAdd:    []string{"a@x.com", "b@x.com"},
			wantRemove: nil,
		},
		{
			name:       "owner never added or removed",
			desired:    []string{owner, "a@x.com"}, // owner listed in config
			current:    []string{owner, "b@x.com"}, // owner present as member
			wantAdd:    []string{"a@x.com"},
			wantRemove: []string{"b@x.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := toSet(tc.current)
			gotAdd, gotRemove := diffCollaborators(tc.desired, current, owner)
			if !sameSet(gotAdd, tc.wantAdd) {
				t.Errorf("toAdd = %v, want %v", gotAdd, tc.wantAdd)
			}
			if !sameSet(gotRemove, tc.wantRemove) {
				t.Errorf("toRemove = %v, want %v", gotRemove, tc.wantRemove)
			}
		})
	}
}

func sameSet(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, ",") == strings.Join(b, ",")
}
