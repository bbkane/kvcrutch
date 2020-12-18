// +build ignore

package main

import (
	"log"
	"path/filepath"

	"github.com/bbkane/kvcrutch/projectroot"
	"github.com/bbkane/kvcrutch/static"
	"github.com/shurcooL/vfsgen"
)

func main() {
	var err error
	err = vfsgen.Generate(static.Static, vfsgen.Options{
		Filename: filepath.Join(
			projectroot.ProjectRootDir(),
			"static/static_vfsdata.go"),
		PackageName:  "static",
		BuildTags:    "dist",
		VariableName: "Static",
	})
	if err != nil {
		log.Fatalln(err)
	}
}
