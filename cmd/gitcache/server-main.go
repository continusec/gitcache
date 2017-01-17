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
	"net"
	"net/http"

	homedir "github.com/mitchellh/go-homedir"

	"github.com/continusec/gitcache"
)

func makeHandleFetch(cacheDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := gitcache.FetchLatest(r.FormValue("repo"),
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
	http.HandleFunc("/fetch", makeHandleFetch(cacheDir))

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
	)

	flag.StringVar(&cacheDir, "cachedir", "~/.gitcache", "Directory to use for caching. May get quite large")
	flag.StringVar(&webBind, "webbind", ":9091", "Binding for webserver.")
	flag.StringVar(&listenProtocol, "protocol", "tcp4", "Listen on tcp or tcp4")

	flag.Parse()

	cacheDir, err := homedir.Expand(cacheDir)
	if err != nil {
		log.Fatal("homedir.Expand: ", err)
	}

	err = runServer(listenProtocol, webBind, cacheDir)
	if err != nil {
		log.Fatal("Error: ", err)
	}

}
