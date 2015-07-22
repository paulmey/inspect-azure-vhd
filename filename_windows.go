package main

import (
	"strings"
)

func fixFilename(name string) string {
	name = strings.Replace(name, ":", "_", -1)
	return name
}
