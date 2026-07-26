//go:build !windows
// +build !windows

package daemon

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/djylb/nps/lib/appconfig"
	"github.com/djylb/nps/lib/common"
)

func init() {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGUSR1)
	go func() {
		for {
			<-s
			_ = appconfig.LoadAppConfig("ini", filepath.Join(common.GetRunPath(), "conf", "nps.conf"))
		}
	}()
}
