// Package appconfig provides the global application configuration accessor.
//
// It replaces beego.AppConfig so that the NPS server no longer depends on the
// beego web framework (router / session / template / captcha). The ini parsing
// itself is delegated to github.com/beego/beego/config, a standalone subpackage
// that pulls in nothing but the standard library, so the on-disk format of
// conf/nps.conf keeps behaving exactly as before: keys and section names are
// lower-cased, ${ENV||default} placeholders are expanded, and `include "x.conf"`
// still works.
package appconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/beego/beego/config"
)

// defaultRunMode mirrors beego's PROD constant, used until conf/nps.conf
// declares a runmode of its own.
const defaultRunMode = "prod"

// Configer is the subset of the configuration API used across NPS. It matches
// beego.AppConfig's method set so call sites only need their import changed.
type Configer interface {
	Set(key, val string) error
	String(key string) string
	Strings(key string) []string
	Int(key string) (int, error)
	Int64(key string) (int64, error)
	Bool(key string) (bool, error)
	Float(key string) (float64, error)
	DefaultString(key string, defaultVal string) string
	DefaultStrings(key string, defaultVal []string) []string
	DefaultInt(key string, defaultVal int) int
	DefaultInt64(key string, defaultVal int64) int64
	DefaultBool(key string, defaultVal bool) bool
	DefaultFloat(key string, defaultVal float64) float64
	DIY(key string) (interface{}, error)
	GetSection(section string) (map[string]string, error)
	SaveConfigFile(filename string) error
}

var (
	// appConfig holds the live *container. It is swapped atomically so that a
	// `nps reload` racing with a config read cannot observe a torn value.
	appConfig atomic.Value // *container

	// configPath remembers where the config was loaded from, for reload and for
	// writing generated values (such as api_jwt_key) back to disk.
	configPath atomic.Value // string
)

func init() {
	appConfig.Store(&container{inner: config.NewFakeConfig(), runMode: defaultRunMode})
	configPath.Store("")
}

// AppConfig returns the current configuration. Before LoadAppConfig succeeds it
// returns an empty configuration rather than nil, so early reads fall back to
// their defaults instead of panicking.
func AppConfig() Configer {
	return appConfig.Load().(*container)
}

// Path returns the absolute path the configuration was loaded from, or "" if it
// has not been loaded yet.
func Path() string {
	p, _ := configPath.Load().(string)
	return p
}

// RunMode returns the active run mode, which prefixes every lookup.
func RunMode() string {
	return appConfig.Load().(*container).runMode
}

// LoadAppConfig parses configPath with the named adapter ("ini") and installs
// the result as the global configuration.
func LoadAppConfig(adapterName, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if st, err := os.Stat(abs); err != nil || st.IsDir() {
		if err == nil {
			err = errors.New("is a directory")
		}
		return errors.New("the target config file: " + path + " can not be read: " + err.Error())
	}
	inner, err := config.NewConfig(adapterName, abs)
	if err != nil {
		return err
	}
	appConfig.Store(&container{inner: inner, runMode: resolveRunMode(inner)})
	configPath.Store(abs)
	return nil
}

// resolveRunMode mirrors beego: the BEEGO_RUNMODE environment variable wins,
// then a `runmode` key in the config, then "prod".
func resolveRunMode(inner config.Configer) string {
	if mode := os.Getenv("BEEGO_RUNMODE"); mode != "" {
		return mode
	}
	if mode := inner.String("runmode"); mode != "" {
		return mode
	}
	return defaultRunMode
}

// container applies the run-mode prefix in front of the underlying config,
// exactly like beego's own beegoAppConfig wrapper: `<runmode>::<key>` is tried
// first and a miss falls through to the bare key in the default section.
type container struct {
	inner   config.Configer
	runMode string
}

func (c *container) scoped(key string) string {
	return c.runMode + "::" + key
}

func (c *container) Set(key, val string) error {
	if err := c.inner.Set(c.scoped(key), val); err != nil {
		return c.inner.Set(key, val)
	}
	return nil
}

func (c *container) String(key string) string {
	if v := c.inner.String(c.scoped(key)); v != "" {
		return v
	}
	return c.inner.String(key)
}

func (c *container) Strings(key string) []string {
	if v := c.inner.Strings(c.scoped(key)); len(v) > 0 {
		return v
	}
	return c.inner.Strings(key)
}

func (c *container) Int(key string) (int, error) {
	if v, err := c.inner.Int(c.scoped(key)); err == nil {
		return v, nil
	}
	return c.inner.Int(key)
}

func (c *container) Int64(key string) (int64, error) {
	if v, err := c.inner.Int64(c.scoped(key)); err == nil {
		return v, nil
	}
	return c.inner.Int64(key)
}

func (c *container) Bool(key string) (bool, error) {
	if v, err := c.inner.Bool(c.scoped(key)); err == nil {
		return v, nil
	}
	return c.inner.Bool(key)
}

func (c *container) Float(key string) (float64, error) {
	if v, err := c.inner.Float(c.scoped(key)); err == nil {
		return v, nil
	}
	return c.inner.Float(key)
}

func (c *container) DefaultString(key string, defaultVal string) string {
	if v := c.String(key); v != "" {
		return v
	}
	return defaultVal
}

func (c *container) DefaultStrings(key string, defaultVal []string) []string {
	if v := c.Strings(key); len(v) != 0 {
		return v
	}
	return defaultVal
}

func (c *container) DefaultInt(key string, defaultVal int) int {
	if v, err := c.Int(key); err == nil {
		return v
	}
	return defaultVal
}

func (c *container) DefaultInt64(key string, defaultVal int64) int64 {
	if v, err := c.Int64(key); err == nil {
		return v
	}
	return defaultVal
}

func (c *container) DefaultBool(key string, defaultVal bool) bool {
	if v, err := c.Bool(key); err == nil {
		return v
	}
	return defaultVal
}

func (c *container) DefaultFloat(key string, defaultVal float64) float64 {
	if v, err := c.Float(key); err == nil {
		return v
	}
	return defaultVal
}

func (c *container) DIY(key string) (interface{}, error) {
	return c.inner.DIY(key)
}

func (c *container) GetSection(section string) (map[string]string, error) {
	return c.inner.GetSection(strings.ToLower(section))
}

func (c *container) SaveConfigFile(filename string) error {
	return c.inner.SaveConfigFile(filename)
}
