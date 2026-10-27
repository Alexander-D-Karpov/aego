package update

import (
	"errors"

	"aego/engine/module"
)

var errBrowser = errors.New("updates are not available in the browser")

// browser is the web build's backend: the page always serves the latest build, so State stays idle.
type browser struct{}

func (browser) init(*Module, *module.InitContext) error { return nil }

func (browser) check(*Module) error { return errBrowser }

func (browser) download(*Module) {}

func (browser) apply(*Module) error { return errBrowser }
