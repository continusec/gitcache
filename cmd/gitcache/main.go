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
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
)

var (
	CacheDir       string
	WebBind        string
	ListenProtocol string
)

func makeCommand(cmd string, args ...string) *exec.Cmd {
	log.Println(cmd, strings.Join(args, " "))

	return exec.Command(cmd, args...)
}

func handleFetch(w http.ResponseWriter, r *http.Request) {
	repo := r.FormValue("repo")
	if len(repo) == 0 {
		http.Error(w, "Must specify repo", 400)
		return
	}
	branch := r.FormValue("branch")
	if len(branch) == 0 {
		http.Error(w, "Must specify branch, even if you know the commit (we may need it to fetch)", 400)
		return
	}
	commit := r.FormValue("commit") // optional
	tree := r.FormValue("tree")     // optional
	format := r.FormValue("format") // required
	if len(format) == 0 {
		http.Error(w, "Must specify format, e.g. tgz", 400)
		return
	}

	// First, make sure workspace exists
	hash := sha256.Sum256([]byte(repo))
	gd := path.Join(CacheDir, hex.EncodeToString(hash[:]))
	_, err := os.Stat(gd)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(gd, 0755)
		if err == nil {
			err = makeCommand("git", "--git-dir", gd, "init", "--bare").Run()
		}
	}
	if err != nil {
		log.Print("Error creating git dir: ", gd, err)
		http.Error(w, "Cannot create git dir", 500)
		return
	}

	// If no commit is specified, fetch latest and set.
	haveFetched := false
	if len(commit) == 0 {
		err = makeCommand("git", "--git-dir", gd, "fetch", repo, "+"+branch+":"+branch).Run()
		if err != nil {
			http.Error(w, "Error fetching from repo", 502)
			return
		}
		haveFetched = true

		commitHex, err := makeCommand("git", "--git-dir", gd, "rev-parse", branch).Output()
		if err != nil {
			http.Error(w, "Error fetching latest commit from repo", 502)
			return
		}

		commit = strings.TrimSpace(string(commitHex))
	}

	// Optimistically try, will fail if we don't have the commit, but it's cheap to try
	cmd := makeCommand("git", "--git-dir", gd, "archive", "--format", "tar", commit+":"+tree)
	cmd.Stdout = w
	err = cmd.Run()
	if err != nil && !haveFetched {
		// If we haven't fetched already, try one more time
		err = makeCommand("git", "--git-dir", gd, "fetch", repo, "+"+branch+":"+branch).Run()
		if err != nil {
			http.Error(w, "Error fetching from repo", 502)
			return
		}

		haveFetched = true

		cmd := makeCommand("git", "--git-dir", gd, "archive", "--format", format, commit+":"+tree)
		cmd.Stdout = w
		err = cmd.Run()
	}

	if err != nil {
		// may be too late, but try to write error code
		http.Error(w, "Error running archive", 502)
		return
	}
}

func main() {
	flag.StringVar(&CacheDir, "cachedir", "/tmp/gitcache", "Directory to use for caching.")
	flag.StringVar(&WebBind, "webbind", ":9091", "Binding for webserver.")
	flag.StringVar(&ListenProtocol, "protocol", "tcp4", "Listen on tcp or tcp4")
	flag.Parse()

	http.HandleFunc("/fetch", handleFetch) // set router

	ln, err := net.Listen(ListenProtocol, WebBind) // explicit listener since we want ipv4 today
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

	log.Print("Serving on ", WebBind)

	err = http.Serve(ln, nil)
	if err != nil {
		log.Fatal("Serve: ", err)
	}
}
