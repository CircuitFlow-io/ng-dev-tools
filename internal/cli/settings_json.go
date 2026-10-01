package cli

import "github.com/CircuitFlow-io/ng-dev-tools/internal/settings"

type settingsJSON struct {
	File     string        `json:"file"`
	Settings []settingJSON `json:"settings"`
}

type settingJSON struct {
	Name string `json:"name"`
	// Value is the effective value: the default when the setting is not set, "" without one.
	Value       string `json:"value"`
	IsSet       bool   `json:"isSet"`
	Description string `json:"description"`
}

func toSettingsJSON(file string, s settings.Settings, home string) settingsJSON {
	views := make([]settingJSON, 0, len(settings.Keys))
	for _, k := range settings.Keys {
		views = append(views, toSettingJSON(k, s, home))
	}
	return settingsJSON{File: file, Settings: views}
}

func toSettingJSON(k settings.Key, s settings.Settings, home string) settingJSON {
	return settingJSON{Name: k.Name, Value: k.Value(s, home), IsSet: k.IsSet(s), Description: k.Description}
}
