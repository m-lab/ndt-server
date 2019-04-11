// Package platformx contains platform specific code
package platformx

// WarnIfNotFullySupported will emit a warning if the platform is not
// fully supported by github.com/m-lab/ndt-server.
func WarnIfNotFullySupported() {
	maybeEmitWarning()
}
