//go:build !darwin && !windows

package sysproxy

// settings is no proxy: elsewhere the environment's HTTPS_PROXY says it.
func settings() config { return config{} }
