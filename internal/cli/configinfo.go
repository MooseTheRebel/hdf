package cli

import "os"

// ConfigInfo is the result of computeConfigInfo: the config file's path and
// raw contents (or an indication it doesn't exist yet), as shown by both
// `hdf config` and the GUI's config view.
type ConfigInfo struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Content string `json:"content"`
}

// computeConfigInfo reads the config file at cfgPath. A missing file is not
// an error — it returns Exists: false, matching `hdf config`'s existing
// friendly "no config found" message. Any other read error (e.g.
// permissions) is returned unwrapped, matching the CLI's current behavior
// of surfacing the raw os.ReadFile error.
func computeConfigInfo(cfgPath string) (*ConfigInfo, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ConfigInfo{Path: cfgPath, Exists: false}, nil
		}
		return nil, err
	}
	return &ConfigInfo{Path: cfgPath, Exists: true, Content: string(data)}, nil
}
