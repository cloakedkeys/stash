// Package build provides the version information for the application.
package build

import (
	"regexp"
)

var version string
var buildstamp string
var githash string
var officialBuild string

// updateRepo is the GitHub "owner/repo" that the built-in update check queries.
// It is set at build time via -ldflags. Without it there is no update check,
// since upstream's releases aren't updates to this copy: they would offer an
// older version, with a different database schema.
var updateRepo string

func Version() (string, string, string) {
	return version, githash, buildstamp
}

// UpdateRepo returns the GitHub "owner/repo" used by the update check, or an
// empty string if this build has no update check.
func UpdateRepo() string {
	return updateRepo
}

func VersionString() string {
	var versionString string
	switch {
	case version != "":
		if githash != "" && !IsDevelop() {
			versionString = version + " (" + githash + ")"
		} else {
			versionString = version
		}
	case githash != "":
		versionString = githash
	default:
		versionString = "unknown"
	}
	if IsOfficial() {
		versionString += " - Official Build"
	} else {
		versionString += " - Unofficial Build"
	}
	if buildstamp != "" {
		versionString += " - " + buildstamp
	}
	return versionString
}

func IsOfficial() bool {
	return officialBuild == "true"
}

func IsDevelop() bool {
	if githash == "" {
		return false
	}

	// if the version is suffixed with -x-xxxx, then we are running a development build
	develop := false
	re := regexp.MustCompile(`-\d+-g\w+$`)
	if re.MatchString(version) {
		develop = true
	}
	return develop
}
