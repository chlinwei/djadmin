package main

import (
	"fmt"
	"os"
	"time"

	"autoadmin/internal/identity"
)

func main() {
	manager := identity.NewTokenManager(os.Args[1], time.Hour)
	token, err := manager.Issue(65, "admin", []string{})
	if err != nil { panic(err) }
	fmt.Println(token)
}
