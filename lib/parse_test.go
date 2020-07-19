package gobot

import "testing"

//BenchmarkGetLinks ...
func BenchmarkGetLinks(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, _ = GetLinks("https://www.google.com")
	}
}
