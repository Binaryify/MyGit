module mygit

go 1.27.3

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/egoist/mygo v0.7.2
	github.com/fsnotify/fsnotify v1.9.0
	golang.org/x/sys v0.48.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// mygo v0.7.2, without its docs, site, examples and tests, and with the
// diff's fillers painted only in sight (egoist/mygo#904): the commit after
// the one adding it is the patch. Back to the release once it has the fix.
replace github.com/egoist/mygo => ./third_party/mygo
