package git

import (
	"context"
	"strings"
)

// Identity is who commits are made as: git's user.name and user.email.
type Identity struct {
	Name, Email string
	// Global says the identity is the user's for every repository
	// (--global), rather than this repository's own (--local).
	Global bool
}

// Identity is who the repository's commits are made as now, and where
// that is set: its own config when it sets either, else the user's.
func (r *Repo) Identity(ctx context.Context) Identity {
	local := Identity{Name: r.config(ctx, "--local", "user.name"), Email: r.config(ctx, "--local", "user.email")}
	global := Identity{Name: r.config(ctx, "--global", "user.name"), Email: r.config(ctx, "--global", "user.email"), Global: true}
	if local.Name == "" && local.Email == "" {
		return global
	}
	// Git takes each key from the nearest config setting it.
	if local.Name == "" {
		local.Name = global.Name
	}
	if local.Email == "" {
		local.Email = global.Email
	}
	return local
}

// SetIdentity makes the repository's commits be made as id. A global one
// is set for the user and the repository's own removed, as it would
// override it here.
func (r *Repo) SetIdentity(ctx context.Context, id Identity) error {
	scope := "--local"
	if id.Global {
		scope = "--global"
	}
	for _, kv := range [][2]string{{"user.name", id.Name}, {"user.email", id.Email}} {
		var err error
		if kv[1] == "" {
			err = r.unsetConfig(ctx, scope, kv[0])
		} else {
			_, err = r.Git(ctx, "config", scope, kv[0], kv[1])
		}
		if err != nil {
			return err
		}
		if id.Global {
			if err := r.unsetConfig(ctx, "--local", kv[0]); err != nil {
				return err
			}
		}
	}
	return nil
}

// config is a key's value in one scope, or as git takes it for scope "",
// "" when it isn't set there.
func (r *Repo) config(ctx context.Context, scope, key string) string {
	args := []string{"config", "--get", key}
	if scope != "" {
		args = []string{"config", scope, "--get", key}
	}
	out, _ := r.Git(ctx, args...)
	return strings.TrimSpace(out)
}

// unsetConfig removes a key from one scope, set there or not.
func (r *Repo) unsetConfig(ctx context.Context, scope, key string) error {
	if r.config(ctx, scope, key) == "" {
		return nil // git exits 5 unsetting what isn't set
	}
	_, err := r.Git(ctx, "config", scope, "--unset-all", key)
	return err
}

// NoIdentity says err is git refusing to commit as nobody: the user has
// set no name or email anywhere.
func NoIdentity(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Please tell me who you are") ||
		strings.Contains(err.Error(), "empty ident name"))
}
