package project

import (
	"aego/core/ids"
	"errors"
)

func ChangeMainScene(p *Project, before, after ids.AssetID) error {
	if p.MainScene != before {
		return errors.New("the main scene changed; refresh the project settings")
	}
	p.MainScene = after
	if err := p.Save(); err != nil {
		p.MainScene = before
		return err
	}
	return nil
}
