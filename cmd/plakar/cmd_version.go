package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/poolpOrg/plakar/store"
)

func cmd_version(pstore store.Store, args []string) {
	if len(args) != 0 {
		log.Fatalf("%s: no parameter expected for version", flag.CommandLine.Name())
	}

	fmt.Println(VERSION)
}
