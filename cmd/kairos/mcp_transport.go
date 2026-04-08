package main

import "strings"

func transportIncludes(transport, target string) bool {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "", "both":
		return true
	case target:
		return true
	default:
		return false
	}
}
