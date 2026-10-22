// Package buildinfo reports which build of Loom is running.
package buildinfo

import "runtime/debug"

// Version is the module version for released builds. Dev builds report
// "devel" plus the VCS revision, commit time and a dirty marker, so any two
// different dev builds compare unequal.
func Version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	s := map[string]string{}
	for _, kv := range bi.Settings {
		s[kv.Key] = kv.Value
	}
	v := "devel"
	if r := s["vcs.revision"]; r != "" {
		v += "+" + r
	}
	if t := s["vcs.time"]; t != "" {
		v += "." + t
	}
	if s["vcs.modified"] == "true" {
		v += ".dirty"
	}
	return v
}
