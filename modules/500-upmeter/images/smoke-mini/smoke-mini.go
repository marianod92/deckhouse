/*
Copyright 2021 Flant JSC

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
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	listenHost = "0.0.0.0"
	listenPort = "8080"

	// targetServices is the list of neighbor indexes, e.g. for "c" it is []string{"a", "b", "d", "e"}
	targetServices      = strings.Split(os.Getenv("SMOKE_MINI_STS_LIST"), " ")
	clusterIpServiceUrl = "http://smoke-mini-cluster-ip:8080"

	serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	// state for shutdown
	ready = true
)

func podHost(x string) string {
	return fmt.Sprintf("smoke-mini-%s-0", x)
}

func singleTargetServiceURL(x string) string {
	return fmt.Sprintf("http://smoke-mini-%s:%s/", x, listenPort)
}

func init() {
	log.SetFormatter(&log.JSONFormatter{})
}

func main() {
	s := &http.Server{
		Handler: setupHandlers(),
		Addr:    listenHost + ":" + listenPort,
	}

	go func() {
		err := s.ListenAndServe()
		if err == nil || err == http.ErrServerClosed {
			log.Info("Shutting down.")
			return
		}
		log.Error(err)
	}()

	// Block to wait for a signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	sig := <-sigs

	// 30 sec is the readiness check timeout
	deadline := time.Now().Add(30 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	ready = false

	log.Info("Got signal ", sig)
	err := s.Shutdown(ctx)
	if err != nil {
		log.Error(err)
	}
}

func setupHandlers() *http.ServeMux {
	mux := http.NewServeMux()

	// k8s
	mux.HandleFunc("/ready", readyHandler)

	// upmeter probes
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/dns", dnsHandler)
	mux.HandleFunc("/neighbor", neighborHandler)
	mux.HandleFunc("/neighbor-via-service", neighborViaServiceHandler)

	return mux
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	if ready {
		w.WriteHeader(200)
	} else {
		w.WriteHeader(500)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	log.Info(os.Getenv("HOSTNAME"), r.RemoteAddr, r.RequestURI)

	if r.RequestURI != "/" {
		w.WriteHeader(404)
		fmt.Fprintf(w, "404 Not Found %s\n", r.RequestURI)
		return
	}
	fmt.Fprintf(w, "ok")
}

func dnsHandler(w http.ResponseWriter, r *http.Request) {
	log.Info(r.RemoteAddr, r.RequestURI)
	_, err := net.LookupIP("kubernetes.default")
	if err != nil {
		w.WriteHeader(500)
		log.Error(err)
		return
	}
	fmt.Fprintf(w, "ok")
}

// neighborHandler checks other smoke-mini pods availabilty. It gets responses from pods via
// headless single-instance service URL, e.g. for "c" it would be "http://smoke-mini-c:<port>", which
// targets single pod "smoke-mini-c-0".
func neighborHandler(w http.ResponseWriter, r *http.Request) {
	log.Info(r.RemoteAddr, r.RequestURI)
	for i := len(targetServices) - 1; i >= 0; i-- {
		if podHost(targetServices[i]) == os.Getenv("HOSTNAME") {
			targetServices = append(targetServices[:i], targetServices[i+1:]...)
		}
	}
	client := http.Client{
		Timeout: 2 * time.Second,
	}
	errorCount := 0
	for i := 0; i < len(targetServices); i++ {
		if errorCount <= 2 {
			resp, err := client.Get(singleTargetServiceURL(targetServices[i]))
			if err != nil {
				log.Error(err)
				errorCount++
				continue
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil || string(body) != "ok" {
				log.Error(err)
				errorCount++
			}
		} else {
			w.WriteHeader(500)
			return
		}
	}
	fmt.Fprintf(w, "ok")
}

// neighborViaServiceHandler checks the availabilty of any smoke-mini pod including themselves via
// service "cluster IP", i.e. via iptables rules. In worst case, the pod gets all responses from
// itself.
func neighborViaServiceHandler(w http.ResponseWriter, r *http.Request) {
	maxErrors := 2

	log.Info(r.RemoteAddr, r.RequestURI)
	client := http.Client{Timeout: 2 * time.Second}

	errorCount := 0
	for i := 0; i < len(targetServices)-1; i++ {
		if errorCount <= maxErrors {
			resp, err := client.Get(clusterIpServiceUrl)
			if err != nil {
				log.Error(err)
				errorCount++
				continue
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil || string(body) != "ok" {
				log.Error(err)
				errorCount++
			}
		} else {
			w.WriteHeader(500)
			return
		}
	}

	fmt.Fprintf(w, "ok")
}
