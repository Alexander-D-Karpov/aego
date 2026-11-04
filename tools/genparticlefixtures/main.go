package main

import (
	"flag"
	"log"

	"aego/tests/particlefixture"
)

func main() {
	out := flag.String("out", ".cache/particle-goldens/project", "reference project for native and browser particle tests")
	flag.Parse()
	if err := particlefixture.Write(*out); err != nil {
		log.Fatal(err)
	}
}
