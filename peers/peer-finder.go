/*
Copyright 2014 The Kubernetes Authors.
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

// A small utility program to lookup hostnames of endpoints in a service.
package peers

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fzerorubigd/k8s-netdata-proxy/sets"

	"context"
)

const (
	pollPeriod = 1 * time.Second
)

func lookup(svcName string) (sets.String, error) {
	endpoints := sets.NewString()
	_, srvRecords, err := net.LookupSRV("", "", svcName)
	if err != nil {
		return endpoints, err
	}
	for _, srvRecord := range srvRecords {
		// The SRV records ends in a "." for the root domain
		ep := fmt.Sprintf("%v", srvRecord.Target[:len(srvRecord.Target)-1])
		endpoints.Insert(ep)
	}
	return endpoints, nil
}

func Find(ctx context.Context, ns, domain, svc string, onChange chan<- sets.String) {
	if ns == "" {
		ns = os.Getenv("POD_NAMESPACE")
	}
	//hostname, err := os.Hostname()
	//if err != nil {
	//	log.Fatalf("Failed to get hostname: %s", err)
	//}
	var domainName string

	// If domain is not provided, try to get it from resolv.conf
	if domain == "" {
		resolvConfBytes, err := ioutil.ReadFile("/etc/resolv.conf")
		resolvConf := string(resolvConfBytes)
		if err != nil {
			log.Fatal("Unable to read /etc/resolv.conf")
		}

		var re *regexp.Regexp
		if ns == "" {
			// Looking for a domain that looks like with *.svc.**
			re, err = regexp.Compile(`\A(.*\n)*search\s{1,}(.*\s{1,})*(?P<goal>[a-zA-Z0-9-]{1,63}.svc.([a-zA-Z0-9-]{1,63}\.)*[a-zA-Z0-9]{2,63})`)
		} else {
			// Looking for a domain that looks like svc.**
			re, err = regexp.Compile(`\A(.*\n)*search\s{1,}(.*\s{1,})*(?P<goal>svc.([a-zA-Z0-9-]{1,63}\.)*[a-zA-Z0-9]{2,63})`)
		}
		if err != nil {
			log.Fatalf("Failed to create regular expression: %v", err)
		}

		groupNames := re.SubexpNames()
		result := re.FindStringSubmatch(resolvConf)
		for k, v := range result {
			if groupNames[k] == "goal" {
				if ns == "" {
					// Domain is complete if ns is empty
					domainName = v
				} else {
					// Need to convert svc.** into ns.svc.**
					domainName = ns + "." + v
				}
				break
			}
		}
		log.Printf("Determined Domain to be %s", domainName)

	} else {
		domainName = strings.Join([]string{ns, "svc", domain}, ".")
	}

	if svc == "" || domainName == "" {
		log.Fatalf("Incomplete args, require -on-change and/or -on-start, -service and -ns or an env var for POD_NAMESPACE.")
	}

	//myName := strings.Join([]string{hostname, svc, domainName}, ".")
	var err error
	newPeers, peers := sets.NewString(), sets.NewString()
	for {
		select {
		case <-time.After(pollPeriod):
			newPeers, err = lookup(svc)
			if err != nil {
				log.Printf("%v", err)
				continue
			}
			//if newPeers.Equal(peers) || !newPeers.Has(myName) {
			//	log.Printf("Have not found myself in list yet.\nMy Hostname: %s\nHosts in list: %s", myName, strings.Join(newPeers.List(), ", "))
			//	continue
			//}
			//log.Printf("Peer list updated\nwas %v\nnow %v", peers.List(), newPeers.List())

			if peers.Equal(newPeers) {
				// noting to inform
				continue
			}

			onChange <- newPeers
			peers = newPeers
		case <-ctx.Done():
			return
		}
	}
	log.Printf("Peer finder exiting")
}
