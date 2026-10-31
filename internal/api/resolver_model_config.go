package api

import (
	"context"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/plugin"
)

func (r *configResultResolver) Plugins(ctx context.Context, obj *ConfigResult, include []string) (map[string]map[string]interface{}, error) {
	var ret map[string]map[string]interface{}

	if len(include) == 0 {
		ret = config.GetInstance().GetAllPluginConfiguration()
	} else {
		ret = make(map[string]map[string]interface{})

		for _, id := range include {
			c := config.GetInstance().GetPluginConfiguration(id)
			if len(c) > 0 {
				ret[id] = c
			}
		}
	}

	// plugin settings can hold credentials
	if !currentUserIsAdmin(ctx) {
		ret = publicPluginSettings(ret, manager.GetInstance().PluginCache.ListPlugins())
	}

	return ret, nil
}

// publicPluginSettings returns only the settings that the plugins declare as public.
func publicPluginSettings(settings map[string]map[string]interface{}, plugins []*plugin.Plugin) map[string]map[string]interface{} {
	ret := make(map[string]map[string]interface{})

	for _, p := range plugins {
		for _, s := range p.Settings {
			v, found := settings[p.ID][s.Name]
			if !s.Public || !found {
				continue
			}

			if ret[p.ID] == nil {
				ret[p.ID] = make(map[string]interface{})
			}
			ret[p.ID][s.Name] = v
		}
	}

	return ret
}
