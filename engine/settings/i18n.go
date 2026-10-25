package settings

import (
	"sync"

	"aego/engine/i18n"
)

// Catalog keys of the settings texts (D348): setting.<id>, setting.<id>.doc, setting.<id>.<choice>, setting.category.<name>.

type descKeys struct {
	title, doc, en string
	choices        map[string]string
}

var (
	keyMu      sync.RWMutex
	descKeyMap = map[string]*descKeys{}
	catKeyMap  = map[string]string{}
)

func keysOf(d *Desc) *descKeys {
	keyMu.RLock()
	k := descKeyMap[d.ID]
	keyMu.RUnlock()
	if k != nil {
		return k
	}
	k = &descKeys{title: "setting." + d.ID, doc: "setting." + d.ID + ".doc", en: d.Label(), choices: map[string]string{}}
	keyMu.Lock()
	descKeyMap[d.ID] = k
	keyMu.Unlock()
	return k
}

// Text is the setting's title in the current language.
func (d *Desc) Text() string {
	k := keysOf(d)
	return i18n.Lookup(k.title, k.en)
}

// Help is the setting's description in the current language.
func (d *Desc) Help() string {
	if d.Doc == "" {
		return ""
	}
	return i18n.Lookup(keysOf(d).doc, d.Doc)
}

// ChoiceText is a choice's label in the current language; a choice without a label shows its name.
func (d *Desc) ChoiceText(c Choice) string {
	en := c.Label
	if en == "" {
		en = c.Name
	}
	if c.Name == "" {
		return en
	}
	k := keysOf(d)
	keyMu.RLock()
	key, ok := k.choices[c.Name]
	keyMu.RUnlock()
	if !ok {
		key = "setting." + d.ID + "." + c.Name
		keyMu.Lock()
		k.choices[c.Name] = key
		keyMu.Unlock()
	}
	return i18n.Lookup(key, en)
}

// CategoryText is a settings category in the current language.
func CategoryText(c string) string {
	keyMu.RLock()
	key, ok := catKeyMap[c]
	keyMu.RUnlock()
	if !ok {
		key = "setting.category." + c
		keyMu.Lock()
		catKeyMap[c] = key
		keyMu.Unlock()
	}
	return i18n.Lookup(key, c)
}
