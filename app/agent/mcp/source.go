package mcp

import (
	"aego/app/agent"
	"aego/project"
)

// Source runs the tool catalog: the CLI opens a session per call, the editor runs on its own documents (D341).
type Source interface {
	Run(name string, a agent.Args) (*agent.Output, error)
	Paths() ([]string, error)
	Project() *project.Project
	Rel(p string) (string, error)
}

// Extra is a resource a Source adds to the list, such as the editor's selection.
type Extra struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Read        func() (string, error)
}

type kitSource struct {
	kit *agent.Kit
}

func (k kitSource) Run(name string, a agent.Args) (*agent.Output, error) {
	sess, err := k.kit.Open()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	return agent.Run(sess, name, a)
}

func (k kitSource) Paths() ([]string, error) {
	sess, err := k.kit.Open()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	var out []string
	for _, a := range sess.Pipeline.Assets() {
		out = append(out, a.Path)
	}
	return out, nil
}

func (k kitSource) Project() *project.Project {
	return k.kit.Project
}

func (k kitSource) Rel(p string) (string, error) {
	return k.kit.Rel(p)
}
