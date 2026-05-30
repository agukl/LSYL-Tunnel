package main

import (
	"flag"
	"fmt"
	"log"

	"lsyltunnel/src/internal/passutil"
)

func main() {
	password := flag.String("password", "", "password to hash")
	flag.Parse()
	if *password == "" {
		log.Fatal("missing -password")
	}
	hash, err := passutil.HashPassword(*password)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(hash)
}
