package playground

import (
	"io/fs"
	"sync"
	"testing"
)

func TestMemoryUserConcurrentReadWriteAndList(t *testing.T) {
	m := &memUser{files: map[string][]byte{}}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Go(func() {
			<-start
			for i := range 1000 {
				if worker == 0 {
					if err := m.WriteFile("saves/slot", []byte{byte(i)}); err != nil {
						t.Error(err)
					}
					if i%2 == 0 {
						if err := m.Remove("saves/slot"); err != nil {
							t.Error(err)
						}
					}
				} else {
					if _, err := m.ReadFile("saves/slot"); err != nil && err != fs.ErrNotExist {
						t.Error(err)
					}
					if _, err := m.Stat("saves/slot"); err != nil && err != fs.ErrNotExist {
						t.Error(err)
					}
					if _, err := m.List("saves"); err != nil {
						t.Error(err)
					}
				}
			}
		})
	}
	close(start)
	wg.Wait()
}

func TestMemoryUserCopiesBytes(t *testing.T) {
	m := &memUser{files: map[string][]byte{}}
	data := []byte("save")
	if err := m.WriteFile("saves/slot", data); err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	read, err := m.ReadFile("saves/slot")
	if err != nil || string(read) != "save" {
		t.Fatalf("write did not retain a copy: %q %v", read, err)
	}
	read[0] = 'Y'
	read, err = m.ReadFile("saves/slot")
	if err != nil || string(read) != "save" {
		t.Fatalf("read exposed the stored bytes: %q %v", read, err)
	}
}
