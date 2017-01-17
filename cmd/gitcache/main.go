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
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	homedir "github.com/mitchellh/go-homedir"
)

func makeCommand(cmd string, args ...string) *exec.Cmd {
	log.Println(cmd, strings.Join(args, " "))

	return exec.Command(cmd, args...)
}

func fetchUpstream(gd, repo, branch string) error {
	return makeCommand("git", "--git-dir", gd, "fetch", repo, "+"+branch+":"+branch).Run()
}

func sendDownstream(gd, commit, tree string, out io.Writer) error {
	cmd := makeCommand("git", "--git-dir", gd, "archive", "--format", "tar", commit+":"+tree)
	pipeTar, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	tarIn := tar.NewReader(pipeTar)
	tarOut := tar.NewWriter(out)
	defer tarOut.Close()

	for {
		header, err := tarIn.Next()
		if err != nil {
			if err == io.EOF {
				return cmd.Wait() // normal exit point
			} else {
				return err
			}
		}

		// Reset modification time to constant value else we get non-deterministic
		// output from git
		header.ModTime = time.Unix(0, 0)

		err = tarOut.WriteHeader(header)
		if err != nil {
			return err
		}

		written, err := io.CopyN(tarOut, tarIn, header.Size)
		if err != nil {
			return err

		}

		if written != header.Size {
			return err
		}
	}
}

func getHeadCommit(gd, repo, branch string) (string, error) {
	err := fetchUpstream(gd, repo, branch)
	if err != nil {
		return "", err
	}

	commitHex, err := makeCommand("git", "--git-dir", gd, "rev-parse", branch).Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(commitHex)), nil
}

// Return git workspace dir that is ready to go
func preflightAndInit(repo, branch, format, cacheDir string) (string, error) {
	if len(repo) == 0 {
		return "", errors.New("Must specify repo")
	}
	if len(branch) == 0 {
		return "", errors.New("Must specify branch, even if you know the commit (we may need it to fetch)")
	}
	if len(format) == 0 {
		return "", errors.New("Must specify format, e.g. tgz")
	}

	if format != "tar" && format != "tgz" {
		return "", errors.New("Format must be tar or tgz for now")
	}

	// Make sure workspace exists
	hash := sha256.Sum256([]byte(repo))
	gd := path.Join(cacheDir, hex.EncodeToString(hash[:]))
	_, err := os.Stat(gd)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(gd, 0755)
			if err != nil {
				return "", err
			}
			err = makeCommand("git", "--git-dir", gd, "init", "--bare").Run()
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	return gd, nil
}

// If outputDir is "", write to w. Else write file to outpuDir, and name of file to w
func fetchLatest(repo, branch, commit, tree, format, cacheDir string, outputDir string, ourOutput io.Writer) error {
	gd, err := preflightAndInit(repo, branch, format, cacheDir)
	if err != nil {
		return err
	}

	haveFetched := false
	// If no commit is specified, fetch latest and set.
	if len(commit) == 0 {
		commit, err = getHeadCommit(gd, repo, branch)
		if err != nil {
			return err
		}
		haveFetched = true
	}

	var w io.Writer
	if len(outputDir) == 0 {
		w = ourOutput
	} else {
		fpath := filepath.Join(outputDir, commit+"."+format)
		os.Stdout.Write([]byte(fpath))
		f, err := os.Create(fpath)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	if format == "tgz" {
		gzipper := gzip.NewWriter(w)
		defer gzipper.Close()

		w = gzipper
	}

	// Optimistically try, will fail if we don't have the commit, but it's cheap to try
	err = sendDownstream(gd, commit, tree, w)
	if err == nil {
		return nil
	}

	if haveFetched {
		return err
	}

	// If we haven't fetched already, try one more time
	err = fetchUpstream(gd, repo, branch)
	if err != nil {
		return err
	}

	return sendDownstream(gd, commit, tree, w)
}

func makeHandleFetch(cacheDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fetchLatest(r.FormValue("repo"),
			r.FormValue("branch"),
			r.FormValue("commit"),
			r.FormValue("tree"),
			r.FormValue("format"),
			cacheDir, "", w)
		if err != nil {
			log.Println("Error:", err.Error())
			http.Error(w, err.Error(), 400)
		}
	}
}

func runServer(listenProtocol, webBind, cacheDir string) error {
	http.HandleFunc("/fetch", makeHandleFetch(cacheDir)) // set router

	ln, err := net.Listen(listenProtocol, webBind) // explicit listener since we want ipv4 today
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal("net.InterfaceAddrs: ", err)
	}

	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				log.Print("(optional) export GITCACHE=http://" + ipnet.IP.String() + webBind)
			}
		}
	}

	log.Print("Serving on " + webBind)

	return http.Serve(ln, nil)
}

func main() {
	var (
		cacheDir       string
		webBind        string
		listenProtocol string

		repo   string
		tree   string
		branch string
		commit string
		format string

		outDir string
	)

	flag.StringVar(&cacheDir, "cachedir", "~/.gitcache", "Directory to use for caching. May get quite large")
	flag.StringVar(&webBind, "webbind", ":9091", "Binding for webserver.")
	flag.StringVar(&listenProtocol, "protocol", "tcp4", "Listen on tcp or tcp4")

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

	if len(repo) == 0 {
		err = runServer(listenProtocol, webBind, cacheDir)
	} else {
		err = fetchLatest(repo, branch, commit, tree, format, cacheDir, outDir, os.Stdout)
	}

	if err != nil {
		log.Fatal("Error: ", err)
	}

}
