// +build !dist

package static

import (
	"net/http"
	"path/filepath"

	"github.com/bbkane/kvcrutch/projectroot"
)

// Static contains static files (just the config file for now)
var Static http.FileSystem = http.Dir(filepath.Join(projectroot.ProjectRootDir(), "static/static"))
