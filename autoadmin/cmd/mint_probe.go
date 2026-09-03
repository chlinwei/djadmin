package main

import (
	"fmt"
	"os"
	"time"

	"autoadmin/internal/identity"
)

func main() {
	tokens := identity.NewTokenManager(os.Args[1], time.Hour)
	token, err := tokens.Issue(1, "dev-probe", []string{"inspection:view", "inspection:groups:create", "inspection:groups:update", "inspection:tasks:create", "inspection:tasks:run"})
	if err != nil {
		panic(err)
	}
	fmt.Println(token)
}
