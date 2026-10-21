package audio

import (
	"errors"

	"aego/asset"
	"aego/core/ids"
	"aego/data"
	"aego/script/bind"
)

func eventLibrary(c *bind.Ctx) (*data.Library, error) {
	if c.Scene == nil {
		return nil, data.ErrNoSource
	}
	src, ok := c.Scene.Resolver().(asset.DataSource)
	if !ok {
		return nil, data.ErrNoSource
	}
	return data.NewLibrary(c.Scene.Registry(), src), nil
}

func bindEvent(b *bind.Registry) error {
	_, err := bind.DataAccess[Event](b, "AudioEvent", bind.DataLoaders[Event]{
		Record: func(c *bind.Ctx, id ids.AssetID) (*Event, error) {
			l, err := eventLibrary(c)
			if err != nil {
				return nil, err
			}
			return data.Get[Event](l, id)
		},
		Row: func(c *bind.Ctx, id ids.AssetID, row string) (*Event, bool, error) {
			l, err := eventLibrary(c)
			if err != nil {
				return nil, false, err
			}
			return data.Row[Event](l, id, row)
		},
		Rows: func(c *bind.Ctx, id ids.AssetID) (int, error) {
			l, err := eventLibrary(c)
			if err != nil {
				return 0, err
			}
			t, err := l.Table(id)
			if err != nil {
				return 0, err
			}
			if t.Type.FullName != NameEvent {
				return 0, errors.New("table " + id.Short() + " holds " + t.Type.FullName + ", not " + NameEvent)
			}
			return len(t.IDs), nil
		},
	}, bind.Doc("AudioEvent is an audio event asset."))
	return err
}
