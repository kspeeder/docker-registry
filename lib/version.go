package lib

import (
	"fmt"
)

const APPLICATION_NAME = "docker-ls"

// bump this when you cut a release tag so clients can match the library version
var staticVersion string = "v0.5.2"
var dynamicVersion *string

var dynamicShortVersion *string

func Version() string {
	if dynamicVersion != nil {
		return *dynamicVersion
	} else {
		return staticVersion
	}
}

func ApplicationName() string {
	if dynamicShortVersion == nil {
		return APPLICATION_NAME
	}

	return fmt.Sprintf("%s/%s", APPLICATION_NAME, *dynamicShortVersion)
}
