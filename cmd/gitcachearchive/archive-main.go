/*

Copyright 2017 Continusec Pty Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

*/

package main

import (
	"flag"
	"log"
	"os"

	homedir "github.com/mitchellh/go-homedir"

	"github.com/continusec/gitcache"
)

func main() {
	var (
		cacheDir string

		repo   string
		tree   string
		branch string
		commit string
		format string

		outDir string
	)

	flag.StringVar(&cacheDir, "cachedir", "~/.gitcache", "Directory to use for caching. May get quite large")

	flag.StringVar(&repo, "repo", "", "Repository to fetch from, required or a server will start instead.")
	flag.StringVar(&tree, "tree", "", "Tree to filter, by default get all")
	flag.StringVar(&branch, "branch", "", "Required, branch containing commit")
	flag.StringVar(&commit, "commit", "", "Optional - if not specified will always contact server")
	flag.StringVar(&outDir, "outdir", ".", "Directory to write output.")
	flag.StringVar(&format, "format", "tgz", "tar or tgz")

	flag.Parse()

	cacheDir, err := homedir.Expand(cacheDir)
	if err != nil {
		log.Fatal("homedir.Expand: ", err)
	}

	err = gitcache.FetchLatest(repo, branch, commit, tree, format, cacheDir, outDir, os.Stdout)
	if err != nil {
		log.Fatal("Error: ", err)
	}

}
