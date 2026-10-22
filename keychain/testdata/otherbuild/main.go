// Command otherbuild saves a token as another build of a tool would: as a
// separate program, so the Keychain trusts it and not the test binary.
package main

import (
	"log"
	"os"

	"github.com/nyactl/approvekit/keychain"
)

func main() {
	if err := keychain.New(os.Args[1]).Set(os.Args[2], os.Args[3]); err != nil {
		log.Fatal(err)
	}
}
